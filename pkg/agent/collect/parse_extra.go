package collect

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// ParseNetRoute returns the interface carrying the default route, from
// /proc/net/route. A default route is the line whose Destination is "00000000";
// the loopback interface's own 0.0.0.0 entry is excluded.
func ParseNetRoute(b []byte) (string, error) {
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[0] == "Iface" { // header
			continue
		}
		if fields[1] == "00000000" && fields[0] != "lo" {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("net route: no default route")
}

// ParseMounts returns the device backing the root filesystem ("/"), from
// /proc/mounts.
func ParseMounts(b []byte) (string, error) {
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if fields[1] == "/" {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("mounts: no root mount")
}

// BaseDeviceName maps a /dev path to the base diskstats device name, stripping
// the partition suffix: "/dev/sda1" -> "sda", "/dev/nvme0n1p1" -> "nvme0n1",
// "/dev/mmcblk0p2" -> "mmcblk0". Unrecognized layouts (LVM dm-*, containers)
// are not resolved and return an error; callers treat that as "skip disk I/O".
func BaseDeviceName(dev string) (string, error) {
	name := strings.TrimPrefix(dev, "/dev/")
	if name == dev || name == "" {
		return "", fmt.Errorf("unrecognized device %q", dev)
	}
	// Partition suffixes differ by device family:
	//   - nvme/mmcblk append "p<number>" ("nvme0n1p1" -> "nvme0n1")
	//   - sd/vd/xvd append a bare number ("sda1" -> "sda")
	// For nvme/mmcblk the trailing number is otherwise part of the device name
	// (e.g. nvme0n1), so only a "p<number>" suffix is stripped there.
	if strings.HasPrefix(name, "nvme") || strings.HasPrefix(name, "mmcblk") {
		j := len(name)
		for j > 0 && name[j-1] >= '0' && name[j-1] <= '9' {
			j--
		}
		if j > 0 && j < len(name) && name[j-1] == 'p' {
			return name[:j-1], nil
		}
		return name, nil
	}

	j := len(name)
	for j > 0 && name[j-1] >= '0' && name[j-1] <= '9' {
		j--
	}
	if j > 0 && j < len(name) {
		return name[:j], nil
	}
	return name, nil
}

// utmpRecordSize is sizeof(struct utmp) on 64-bit Linux (x86_64 and aarch64).
// ut_type is the leading int16; USER_PROCESS is 7.
const (
	utmpRecordSize  = 384
	utmpUserProcess = 7
)

// ParseUtmpUsers counts currently logged-in user sessions in a /var/run/utmp
// image: records whose ut_type is USER_PROCESS.
func ParseUtmpUsers(b []byte) (int, error) {
	if len(b)%utmpRecordSize != 0 {
		return 0, fmt.Errorf("utmp: size %d not a multiple of %d", len(b), utmpRecordSize)
	}
	count := 0
	for i := 0; i < len(b); i += utmpRecordSize {
		if binary.LittleEndian.Uint16(b[i:i+2]) == utmpUserProcess {
			count++
		}
	}
	return count, nil
}

// ParseOSRelease returns a human-readable OS name from /etc/os-release, e.g.
// "Ubuntu 22.04.3 LTS (Jammy Jellyfish)".
func ParseOSRelease(b []byte) string {
	name, version := "", ""
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"`)
		switch k {
		case "NAME":
			name = v
		case "VERSION":
			version = v
		}
	}
	if version != "" {
		return name + " " + version
	}
	return name
}

// CPUInfo holds the CPU model and logical core count from /proc/cpuinfo.
type CPUInfo struct {
	Model string
	Cores int
}

// ParseCPUInfo reads the CPU model and counts logical cores from /proc/cpuinfo.
// The file's format differs by architecture: x86 exposes "model name", while
// aarch64 has no model line and instead exposes "CPU implementer"/"CPU part"/
// "CPU architecture". We fall back through those so ARM hosts still get a
// non-empty model.
func ParseCPUInfo(b []byte) CPUInfo {
	var info CPUInfo
	var model, hardware, processor, part, arch string
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		switch key {
		case "model name":
			if model == "" {
				model = val
			}
		case "Hardware":
			hardware = val
		case "Processor":
			if processor == "" {
				processor = val
			}
		case "CPU part":
			part = val
		case "CPU architecture":
			arch = val
		case "processor":
			info.Cores++
		}
	}
	info.Model = firstNonEmpty(model, hardware, processor)
	if info.Model == "" && arch != "" {
		info.Model = "ARMv" + arch
		if part != "" {
			info.Model += " (" + part + ")"
		}
	}
	return info
}

// firstNonEmpty returns the first non-empty string in vals, or "" if all are
// empty.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
