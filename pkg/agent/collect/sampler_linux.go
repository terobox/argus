//go:build linux

package collect

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/terobox/argus/pkg/metrics"
)

// Sampler reads Linux /proc and related sources to produce metrics.Sample and
// metrics.HostInfo values. It caches the pieces that change rarely (primary
// NIC, root device, slow metrics) and computes CPU utilization as the delta
// between consecutive /proc/stat reads.
type Sampler struct {
	agentVer string

	prevCPU CPUStat
	haveCPU bool

	lastSlow    time.Time
	diskCap     *metrics.DiskCapacity
	loggedUsers *int

	netIface   string
	diskDevice string
}

// NewSampler returns a Sampler. agentVer is reported in HostInfo.
func NewSampler(agentVer string) *Sampler {
	return &Sampler{agentVer: agentVer}
}

// Sample returns one full sample. Slow fields (disk capacity, logged users) are
// refreshed at most every slowEvery.
func (s *Sampler) Sample(slowEvery time.Duration) (metrics.Sample, error) {
	sm := metrics.Sample{TS: time.Now().Unix()}

	cpu, err := s.cpuUsedPercent()
	if err != nil {
		return sm, err
	}
	sm.CPU.UsedPercent = cpu

	load, err := parseProc("loadavg", ParseLoadavg)
	if err != nil {
		return sm, err
	}
	sm.Load = metrics.Load{Load1: load.Load1, Load5: load.Load5, Load15: load.Load15}

	mem, err := parseProc("meminfo", ParseMeminfo)
	if err != nil {
		return sm, err
	}
	sm.Mem = memBytes(mem)
	sm.Swap = swapBytes(mem)

	net, err := s.net()
	if err != nil {
		return sm, err
	}
	sm.Net = net

	sm.DiskIO = s.diskIO() // best-effort: never fails the sample

	if time.Since(s.lastSlow) >= slowEvery {
		s.refreshSlow()
	}
	sm.DiskCap = s.diskCap
	sm.LoggedUsers = s.loggedUsers

	return sm, nil
}

// HostInfo returns the host's mostly-static identifying information. publicIP
// comes from configuration and is never probed automatically.
func (s *Sampler) HostInfo(publicIP string) (metrics.HostInfo, error) {
	hostname, _ := os.Hostname()

	osName := "Linux"
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		if name := ParseOSRelease(b); name != "" {
			osName = name
		}
	}

	var un syscall.Utsname
	if err := syscall.Uname(&un); err != nil {
		return metrics.HostInfo{}, fmt.Errorf("uname: %w", err)
	}

	mem, err := parseProc("meminfo", ParseMeminfo)
	if err != nil {
		return metrics.HostInfo{}, err
	}
	cpu := parseProcQuiet("cpuinfo", ParseCPUInfo)

	return metrics.HostInfo{
		Hostname: hostname,
		OS:       osName,
		Kernel:   cstr(un.Release[:]),
		Arch:     cstr(un.Machine[:]),
		CPUModel: cpu.Model,
		CPUCores: cpu.Cores,
		MemTotal: int64(mem.MemTotal) * 1024,
		PublicIP: publicIP,
		AgentVer: s.agentVer,
	}, nil
}

// cpuUsedPercent returns the overall CPU busy percentage as the delta of
// aggregate jiffies between consecutive reads. The first call establishes the
// baseline and returns 0.
func (s *Sampler) cpuUsedPercent() (float64, error) {
	cur, err := parseProc("stat", ParseCPUStat)
	if err != nil {
		return 0, err
	}
	if !s.haveCPU {
		s.prevCPU = cur
		s.haveCPU = true
		return 0, nil
	}
	total := cur.Total() - s.prevCPU.Total()
	idle := cur.IdleTotal() - s.prevCPU.IdleTotal()
	s.prevCPU = cur
	if total == 0 {
		return 0, nil
	}
	return clampPercent((1 - float64(idle)/float64(total)) * 100), nil
}

func (s *Sampler) net() (metrics.Net, error) {
	iface, err := s.primaryIface()
	if err != nil {
		return metrics.Net{}, err
	}
	devs, err := parseProc("net/dev", ParseNetDev)
	if err != nil {
		return metrics.Net{}, err
	}
	d, ok := devs[iface]
	if !ok {
		return metrics.Net{}, fmt.Errorf("net: iface %q not found in /proc/net/dev", iface)
	}
	return metrics.Net{RxBytes: d.RxBytes, TxBytes: d.TxBytes}, nil
}

func (s *Sampler) primaryIface() (string, error) {
	if s.netIface != "" {
		return s.netIface, nil
	}
	iface, err := parseProc("net/route", ParseNetRoute)
	if err != nil {
		return "", err
	}
	s.netIface = iface
	return iface, nil
}

// diskIO returns cumulative disk I/O for the root device, or zeros when the
// root device cannot be mapped (LVM, containers) — a best-effort metric.
func (s *Sampler) diskIO() metrics.DiskIO {
	dev, err := s.rootDevice()
	if err != nil {
		return metrics.DiskIO{}
	}
	stats, err := parseProc("diskstats", ParseDiskstats)
	if err != nil {
		return metrics.DiskIO{}
	}
	d, ok := stats[dev]
	if !ok {
		return metrics.DiskIO{}
	}
	return metrics.DiskIO{ReadBytes: d.ReadBytes, WriteBytes: d.WriteBytes}
}

func (s *Sampler) rootDevice() (string, error) {
	if s.diskDevice != "" {
		return s.diskDevice, nil
	}
	rootDev, err := parseProc("mounts", ParseMounts)
	if err != nil {
		return "", err
	}
	base, err := BaseDeviceName(rootDev)
	if err != nil {
		return "", err
	}
	s.diskDevice = base
	return base, nil
}

func (s *Sampler) refreshSlow() {
	s.lastSlow = time.Now()
	if cap, err := statRootFS(); err == nil {
		s.diskCap = cap
	}
	if n, err := loggedUsers(); err == nil {
		s.loggedUsers = &n
	}
}

func statRootFS() (*metrics.DiskCapacity, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err != nil {
		return nil, fmt.Errorf("statfs: %w", err)
	}
	total := int64(st.Blocks) * int64(st.Bsize)
	used := int64(st.Blocks-st.Bfree) * int64(st.Bsize)
	if total <= 0 {
		return nil, fmt.Errorf("statfs: non-positive total")
	}
	return &metrics.DiskCapacity{TotalBytes: total, UsedBytes: used}, nil
}

func loggedUsers() (int, error) {
	b, err := os.ReadFile("/var/run/utmp")
	if err != nil {
		return 0, fmt.Errorf("read utmp: %w", err)
	}
	return ParseUtmpUsers(b)
}

func memBytes(m Meminfo) metrics.Mem {
	const kb = 1024
	return metrics.Mem{
		TotalBytes:     int64(m.MemTotal) * kb,
		AvailableBytes: int64(m.MemAvailable) * kb,
		UsedBytes:      int64(m.MemTotal-m.MemAvailable) * kb,
	}
}

func swapBytes(m Meminfo) metrics.Swap {
	const kb = 1024
	return metrics.Swap{
		TotalBytes: int64(m.SwapTotal) * kb,
		UsedBytes:  int64(m.SwapTotal-m.SwapFree) * kb,
	}
}

// parseProc reads /proc/name and parses it with parse.
func parseProc[T any](name string, parse func([]byte) (T, error)) (T, error) {
	var zero T
	b, err := os.ReadFile("/proc/" + name)
	if err != nil {
		return zero, fmt.Errorf("read /proc/%s: %w", name, err)
	}
	return parse(b)
}

// parseProcQuiet is like parseProc but returns the zero value on any error; use
// for non-essential files.
func parseProcQuiet[T any](name string, parse func([]byte) T) T {
	var zero T
	b, err := os.ReadFile("/proc/" + name)
	if err != nil {
		return zero
	}
	return parse(b)
}

// cstr converts a NUL-terminated int8 array (as in syscall.Utsname) to a string.
func cstr(b []int8) string {
	var buf []byte
	for _, c := range b {
		if c == 0 {
			break
		}
		buf = append(buf, byte(c))
	}
	return string(buf)
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
