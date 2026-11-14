// Package collect parses the Linux kernel's /proc (and related) files into
// typed values. The parsers in this file are pure functions over byte slices
// so they are platform-independent and unit-testable against fixture data on
// any OS; the code that actually opens /proc lives behind a linux build tag.
package collect

import (
	"fmt"
	"strconv"
	"strings"
)

// CPUStat is the aggregate "cpu" line of /proc/stat. Counters are jiffies since
// boot; utilization is computed from the delta between two samples.
type CPUStat struct {
	User, Nice, System, Idle uint64
	Iowait, Irq, Softirq     uint64
	Steal                    uint64
}

// Total returns the sum of all jiffies across all states.
func (c CPUStat) Total() uint64 {
	return c.User + c.Nice + c.System + c.Idle + c.Iowait + c.Irq + c.Softirq + c.Steal
}

// IdleTotal returns jiffies spent idle or waiting on I/O, the two states
// conventionally excluded from "CPU busy" percentages.
func (c CPUStat) IdleTotal() uint64 { return c.Idle + c.Iowait }

// ParseCPUStat parses the first "cpu " (aggregate) line of /proc/stat. Lines
// for individual cores ("cpu0", "cpu1", …) are ignored.
func ParseCPUStat(b []byte) (CPUStat, error) {
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		// fields[0]=="cpu"; the rest are user nice system idle iowait irq softirq steal …
		if len(fields) < 5 {
			return CPUStat{}, fmt.Errorf("cpu stat: malformed line %q", line)
		}
		vals := make([]uint64, 0, 8)
		for _, f := range fields[1:] {
			v, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				return CPUStat{}, fmt.Errorf("cpu stat: field %q: %w", f, err)
			}
			vals = append(vals, v)
		}
		var c CPUStat
		c.User, c.Nice, c.System, c.Idle = vals[0], vals[1], vals[2], vals[3]
		if len(vals) > 4 {
			c.Iowait = vals[4]
		}
		if len(vals) > 5 {
			c.Irq = vals[5]
		}
		if len(vals) > 6 {
			c.Softirq = vals[6]
		}
		if len(vals) > 7 {
			c.Steal = vals[7]
		}
		return c, nil
	}
	return CPUStat{}, fmt.Errorf("cpu stat: no aggregate 'cpu ' line")
}

// Meminfo holds the /proc/meminfo values argus needs, in kibibytes (kB) as
// reported by the kernel.
type Meminfo struct {
	MemTotal, MemAvailable uint64
	SwapTotal, SwapFree    uint64
}

// ParseMeminfo parses /proc/meminfo. MemTotal is required; MemAvailable and the
// swap figures are best-effort (a swap-less system reports no swap keys).
func ParseMeminfo(b []byte) (Meminfo, error) {
	var m Meminfo
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue // skip malformed lines rather than failing the whole file
		}
		switch fields[0] {
		case "MemTotal:":
			m.MemTotal = val
		case "MemAvailable:":
			m.MemAvailable = val
		case "SwapTotal:":
			m.SwapTotal = val
		case "SwapFree:":
			m.SwapFree = val
		}
	}
	if m.MemTotal == 0 {
		return m, fmt.Errorf("meminfo: missing MemTotal")
	}
	return m, nil
}

// LoadAvg holds the 1/5/15-minute load averages from /proc/loadavg.
type LoadAvg struct{ Load1, Load5, Load15 float64 }

// ParseLoadavg parses /proc/loadavg (first three fields).
func ParseLoadavg(b []byte) (LoadAvg, error) {
	var l LoadAvg
	n, err := fmt.Sscanf(strings.TrimSpace(string(b)), "%f %f %f", &l.Load1, &l.Load5, &l.Load15)
	if err != nil {
		return l, fmt.Errorf("loadavg: %w", err)
	}
	if n != 3 {
		return l, fmt.Errorf("loadavg: parsed %d of 3 fields", n)
	}
	return l, nil
}

// ParseUptimeSeconds parses the first field of /proc/uptime (seconds since
// boot).
func ParseUptimeSeconds(b []byte) (float64, error) {
	fields := strings.Fields(string(b))
	if len(fields) < 1 {
		return 0, fmt.Errorf("uptime: empty")
	}
	up, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("uptime: %w", err)
	}
	return up, nil
}

// NetDev holds the cumulative byte counters for one interface, from
// /proc/net/dev.
type NetDev struct{ RxBytes, TxBytes uint64 }

// ParseNetDev parses /proc/net/dev into a map keyed by interface name. Header
// lines (no colon) are skipped.
func ParseNetDev(b []byte) (map[string]NetDev, error) {
	out := make(map[string]NetDev)
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, ":") {
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		// Receive bytes is the first field; transmit bytes is field[8].
		if len(fields) < 9 {
			continue
		}
		rx, err1 := strconv.ParseUint(fields[0], 10, 64)
		tx, err2 := strconv.ParseUint(fields[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out[strings.TrimSpace(name)] = NetDev{RxBytes: rx, TxBytes: tx}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("netdev: no interfaces")
	}
	return out, nil
}

// DiskStat holds the cumulative bytes read/written for one device, from
// /proc/diskstats. Sector counts are converted to bytes (a sector is 512 bytes).
type DiskStat struct{ ReadBytes, WriteBytes uint64 }

// ParseDiskstats parses /proc/diskstats into a map keyed by device name
// (e.g. "sda", "nvme0n1"). Partition entries share a name prefix with their
// backing device and are filtered by the caller.
func ParseDiskstats(b []byte) (map[string]DiskStat, error) {
	out := make(map[string]DiskStat)
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		// Need at least: major minor name … sectors_read(5) … sectors_written(9).
		if len(fields) < 10 {
			continue
		}
		sectorsRead, err1 := strconv.ParseUint(fields[5], 10, 64)
		sectorsWritten, err2 := strconv.ParseUint(fields[9], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out[fields[2]] = DiskStat{
			ReadBytes:  sectorsRead * 512,
			WriteBytes: sectorsWritten * 512,
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("diskstats: no devices")
	}
	return out, nil
}
