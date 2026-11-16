//go:build linux

package collect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxMajorMinor(t *testing.T) {
	cases := []struct {
		dev          uint64
		major, minor uint32
	}{
		{0, 0, 0},
		{0x800, 8, 0},                    // /dev/sda
		{0xfe00, 254, 0},                 // /dev/vda
		{0x10301, 259, 1},                // the AWS /dev/root node: 259:1 (nvme0n1p1)
		{0x100005623478, 0x1234, 0x5678}, // major and minor above 8 and 0xff
	}
	for _, c := range cases {
		major, minor := linuxMajorMinor(c.dev)
		if major != c.major || minor != c.minor {
			t.Errorf("linuxMajorMinor(%#x) = %d:%d, want %d:%d",
				c.dev, major, minor, c.major, c.minor)
		}
	}
}

func TestMountInfoRootDevs(t *testing.T) {
	// Real mountinfo lines, trimmed to the fields the parser reads
	// (transaction, parent, major:minor, root, mount point, …).
	b := []byte(`25 1 259:1 / / rw,relatime shared:1 - ext4 /dev/root rw
30 1 0:34 / / rw,relatime shared:2 - btrfs /dev/sda2 rw
31 1 0:99 / /home rw,relatime - xfs /dev/sdb1 rw
malformed line
`)
	got := mountInfoRootDevs(b)
	want := [][2]uint32{{259, 1}, {0, 34}}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestIsVirtualDiskName(t *testing.T) {
	for _, name := range []string{
		"loop0", "ram0", "zram0", "sr0", "fd0", "nbd0",
		"dm-0", "md0", "md127", "zd0", "bcache0",
	} {
		if !isVirtualDiskName(name) {
			t.Errorf("isVirtualDiskName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"sda", "sda1", "vda", "xvda", "nvme0n1", "mmcblk0"} {
		if isVirtualDiskName(name) {
			t.Errorf("isVirtualDiskName(%q) = true, want false", name)
		}
	}
}

func TestWholeDiskFromSysfsDir(t *testing.T) {
	disk := t.TempDir()
	if err := os.WriteFile(filepath.Join(disk, "dev"), []byte("8:0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := wholeDiskFromSysfsDir(disk); !ok || got != filepath.Base(disk) {
		t.Errorf("whole disk dir = (%q, %v), want (%q, true)", got, ok, filepath.Base(disk))
	}

	part := filepath.Join(disk, "sda1")
	if err := os.Mkdir(part, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(part, "dev"), []byte("8:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(part, "partition"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := wholeDiskFromSysfsDir(part); !ok || got != filepath.Base(disk) {
		t.Errorf("partition dir = (%q, %v), want (%q, true)", got, ok, filepath.Base(disk))
	}

	// A directory without a "dev" attribute is not a block device.
	if got, ok := wholeDiskFromSysfsDir(t.TempDir()); ok {
		t.Errorf("non-device dir = (%q, true), want (_, false)", got)
	}
}

func TestBlockDeviceByNameRejectsJunk(t *testing.T) {
	for _, name := range []string{"", ".", "..", "no-such-device"} {
		if got, ok := blockDeviceByName(name); ok {
			t.Errorf("blockDeviceByName(%q) = (%q, true), want false", name, got)
		}
	}
}

// TestSysblockPhysicalDisks checks the live system: every disk returned by
// sysblockPhysicalDisks must exist in /proc/diskstats. Skips when /sys/block
// is not accessible (containers without sysfs).
func TestSysblockPhysicalDisks(t *testing.T) {
	disks := sysblockPhysicalDisks()
	if disks == nil {
		t.Skip("/sys/block not available")
	}
	stats, err := parseProc("diskstats", ParseDiskstats)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range disks {
		if _, ok := stats[name]; !ok {
			t.Errorf("sysblock disk %q not found in /proc/diskstats", name)
		}
	}
}

// TestRootBlockDeviceResolvesToDiskstats checks the end-to-end invariant that
// matters: the resolved name must be a /proc/diskstats entry, because that is
// what diskIO indexes. It skips on hosts whose root has no block device
// (containers with overlayfs, ZFS), where zero disk I/O is the honest answer.
func TestRootBlockDeviceResolvesToDiskstats(t *testing.T) {
	name, err := rootBlockDevice()
	if err != nil {
		t.Skipf("no block device backs / here: %v", err)
	}
	stats, err := parseProc("diskstats", ParseDiskstats)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := stats[name]; !ok {
		t.Fatalf("root device %q is not a /proc/diskstats entry", name)
	}
}
