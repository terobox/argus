//go:build linux

package collect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// diskIO returns cumulative disk I/O summed across all hardware-backed block
// devices. Summing all physical disks rather than only the root device means
// machines with a separate data disk — or where root I/O is near-zero — still
// show real workload activity. It is a best-effort metric: errors never fail
// the sample.
//
// Lookup strategy (first success wins):
//  1. /sys/block/<dev>/device — enumerates hardware whole disks; most reliable.
//  2. rootDevice() + diskstats — used when /sys/block is unavailable.
//  3. sumWholeDisks — name-heuristic fallback of last resort.
func (s *Sampler) diskIO() metrics.DiskIO {
	stats, err := parseProc("diskstats", ParseDiskstats)
	if err != nil {
		return metrics.DiskIO{}
	}
	if disks := sysblockPhysicalDisks(); len(disks) > 0 {
		var io metrics.DiskIO
		for _, name := range disks {
			if d, ok := stats[name]; ok {
				io.ReadBytes += d.ReadBytes
				io.WriteBytes += d.WriteBytes
			}
		}
		return io
	}
	if dev, err := s.rootDevice(); err == nil {
		if d, ok := stats[dev]; ok {
			return metrics.DiskIO{ReadBytes: d.ReadBytes, WriteBytes: d.WriteBytes}
		}
	}
	return sumWholeDisks(stats)
}

// sysblockPhysicalDisks returns the kernel names of hardware-backed whole block
// devices from /sys/block/. Two properties make this more reliable than
// filtering /proc/diskstats by name:
//
//  1. /sys/block/ lists only whole disks — partitions (sda1, nvme0n1p1, …)
//     never appear there, so double-counting is structurally impossible.
//  2. The "device" subdirectory exists only for hardware-backed disks. Virtual
//     layers (dm-*, md*, loop*, zram, …) have no such link and are excluded
//     without any name-based guessing.
//
// Returns nil when /sys/block/ is inaccessible (some container environments).
func sysblockPhysicalDisks() []string {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if isVirtualDiskName(name) {
			continue
		}
		if _, err := os.Lstat(filepath.Join("/sys/block", name, "device")); err != nil {
			continue
		}
		out = append(out, name)
	}
	return out
}

// sumWholeDisks adds up the counters of every whole physical disk in
// /proc/diskstats. Partitions are skipped because their I/O is already counted
// by the parent disk, and virtual devices (dm-*, md*, loop, zram, …) are skipped
// so the same I/O is not counted twice at two layers.
func sumWholeDisks(stats map[string]DiskStat) metrics.DiskIO {
	var io metrics.DiskIO
	for name, d := range stats {
		if !isWholePhysicalDisk(name) {
			continue
		}
		io.ReadBytes += d.ReadBytes
		io.WriteBytes += d.WriteBytes
	}
	return io
}

// isWholePhysicalDisk reports whether a diskstats name is a whole disk backed by
// hardware. A partition is identified by its sysfs "partition" attribute, which
// exists for every partition on every driver.
func isWholePhysicalDisk(name string) bool {
	if isVirtualDiskName(name) {
		return false
	}
	_, err := os.Stat(filepath.Join("/sys/class/block", name, "partition"))
	return err != nil
}

// isVirtualDiskName reports whether name belongs to a device family that must
// not be summed as physical hardware.
func isVirtualDiskName(name string) bool {
	for _, prefix := range []string{
		"loop", "ram", "zram", "sr", "fd", "nbd", "dm-", "md", "zd", "bcache",
	} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// rootDevice returns the kernel name of the whole block device backing "/", for
// example "sda", "nvme0n1", "vda", "dm-0", or "md0". The result is cached once
// resolved; a failure is retried on the next sample.
func (s *Sampler) rootDevice() (string, error) {
	if s.diskDevice != "" {
		return s.diskDevice, nil
	}
	name, err := rootBlockDevice()
	if err != nil {
		return "", err
	}
	s.diskDevice = name
	return name, nil
}

// rootBlockDevice resolves the root filesystem to a /proc/diskstats device name.
// Two sources are tried in order:
//
//  1. /proc/self/mountinfo, whose device field for the "/" mount is the
//     filesystem's major:minor. /sys/dev/block maps that number to the kernel's
//     device name without consulting /dev at all, so it works for ext4, XFS, LVM
//     (dm-*), MD RAID (md*) and every sd/vd/nvme/mmcblk layout. A partition
//     resolves to its parent whole disk, whose counters include the partition's
//     I/O.
//  2. The mount source in /proc/mounts, resolved through the same sysfs path.
//     btrfs reports an anonymous device in mountinfo but still names a real
//     /dev node as its source.
//
// Roots with no block device behind them (overlay/tmpfs in containers, ZFS)
// fail both steps; callers treat that as "no disk I/O available".
func rootBlockDevice() (string, error) {
	if b, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		for _, dev := range mountInfoRootDevs(b) {
			if name, ok := blockDeviceFromNumbers(dev[0], dev[1]); ok {
				return name, nil
			}
		}
	}
	src, err := parseProc("mounts", ParseMounts)
	if err != nil {
		return "", err
	}
	if name, ok := blockDeviceFromPath(src); ok {
		return name, nil
	}
	return "", fmt.Errorf("root device %q has no block device in sysfs", src)
}

// mountInfoRootDevs returns the major:minor device of every mountinfo entry
// whose mount point is "/". Stacked mounts can produce more than one entry;
// callers try each until one maps to a block device.
func mountInfoRootDevs(b []byte) [][2]uint32 {
	var out [][2]uint32
	for line := range strings.SplitSeq(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || f[4] != "/" {
			continue
		}
		var major, minor uint32
		if _, err := fmt.Sscanf(f[2], "%d:%d", &major, &minor); err != nil {
			continue
		}
		out = append(out, [2]uint32{major, minor})
	}
	return out
}

// blockDeviceFromPath resolves a mount source such as /dev/sda2,
// /dev/mapper/vg-root, or /dev/root to the kernel name of its whole block
// device. Non-block sources (overlay, tmpfs, zfs dataset names) return false.
func blockDeviceFromPath(dev string) (string, bool) {
	if !strings.HasPrefix(dev, "/dev/") {
		return "", false
	}
	// Prefer the device number when the node exists: it covers symlinks such as
	// /dev/mapper/vg-root and /dev/disk/by-uuid/....
	if resolved, err := filepath.EvalSymlinks(dev); err == nil {
		var st syscall.Stat_t
		if err := syscall.Stat(resolved, &st); err == nil && st.Mode&syscall.S_IFMT == syscall.S_IFBLK {
			major, minor := linuxMajorMinor(uint64(st.Rdev))
			if name, ok := blockDeviceFromNumbers(major, minor); ok {
				return name, true
			}
		}
	}
	// The node may be absent: the hardened agent unit runs with
	// PrivateDevices=true, so its /dev holds only pseudo devices. /proc/mounts
	// still records the kernel name as the source (e.g. /dev/sda2), and
	// /sys/class/block resolves it without touching /dev at all.
	return blockDeviceByName(filepath.Base(dev))
}

// blockDeviceByName maps a kernel block device name (sda2, nvme0n1p1, dm-0) to
// its whole disk through /sys/class/block, which needs no /dev node.
func blockDeviceByName(name string) (string, bool) {
	if name == "" || name == "." || name == ".." {
		return "", false
	}
	dir, err := filepath.EvalSymlinks(filepath.Join("/sys/class/block", name))
	if err != nil {
		return "", false
	}
	return wholeDiskFromSysfsDir(dir)
}

// blockDeviceFromNumbers maps a major:minor to the kernel name of the whole
// block device. /sys/dev/block/<maj>:<min> is a symlink to the device's sysfs
// directory; a partition's directory lives under its whole disk and carries a
// "partition" attribute, so no naming rules (sd/vd/nvme/mmcblk, dm-*, md*) need
// to be hard-coded.
func blockDeviceFromNumbers(major, minor uint32) (string, bool) {
	dir, err := filepath.EvalSymlinks(fmt.Sprintf("/sys/dev/block/%d:%d", major, minor))
	if err != nil {
		return "", false
	}
	return wholeDiskFromSysfsDir(dir)
}

// wholeDiskFromSysfsDir maps a sysfs block-device directory to the name of the
// whole disk that owns it. The "dev" attribute (major:minor) confirms the
// directory really is a block device; a partition is then recognised by its
// "partition" attribute and answered with its parent directory's name.
func wholeDiskFromSysfsDir(dir string) (string, bool) {
	if _, err := os.Stat(filepath.Join(dir, "dev")); err != nil {
		return "", false
	}
	if _, err := os.Stat(filepath.Join(dir, "partition")); err == nil {
		return filepath.Base(filepath.Dir(dir)), true
	}
	return filepath.Base(dir), true
}

// linuxMajorMinor decodes the Linux dev_t encoding held in Stat_t.Rdev into
// major and minor numbers, matching glibc's major() and minor() macros.
func linuxMajorMinor(dev uint64) (major, minor uint32) {
	major = uint32((dev>>8)&0xfff) | uint32((dev>>32)&0xfffff000)
	minor = uint32(dev&0xff) | uint32((dev>>12)&0xffffff00)
	return major, minor
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
