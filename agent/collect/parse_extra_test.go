package collect

import (
	"encoding/binary"
	"testing"
)

func TestParseNetRoute(t *testing.T) {
	fixture := []byte("Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t010110AC\t0003\t0\t0\t0\t00000000\t0\t0\t0\n" +
		"eth0\t000110AC\t00000000\t0001\t0\t0\t0\t00FFFFFF\t0\t0\t0\n" +
		"lo\t00000000\t00000000\t0001\t0\t0\t0\t00000000\t0\t0\t0\n")
	iface, err := ParseNetRoute(fixture)
	if err != nil {
		t.Fatalf("ParseNetRoute: %v", err)
	}
	if iface != "eth0" {
		t.Errorf("iface = %q, want eth0", iface)
	}
}

func TestParseNetRouteNoDefault(t *testing.T) {
	if _, err := ParseNetRoute([]byte("Iface\tDestination\tGateway\n")); err == nil {
		t.Fatal("expected error for missing default route, got nil")
	}
}

func TestParseMounts(t *testing.T) {
	fixture := []byte("proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0\n" +
		"/dev/sda1 / ext4 rw,relatime 0 0\n" +
		"tmpfs /run tmpfs rw,nosuid,nodev 0 0\n")
	dev, err := ParseMounts(fixture)
	if err != nil {
		t.Fatalf("ParseMounts: %v", err)
	}
	if dev != "/dev/sda1" {
		t.Errorf("root device = %q, want /dev/sda1", dev)
	}
}

func TestParseMountsNoRoot(t *testing.T) {
	if _, err := ParseMounts([]byte("proc /proc proc rw 0 0\n")); err == nil {
		t.Fatal("expected error for missing root mount, got nil")
	}
}

func TestParseUtmpUsers(t *testing.T) {
	buf := make([]byte, utmpRecordSize*3)
	binary.LittleEndian.PutUint16(buf[0:2], utmpUserProcess)
	binary.LittleEndian.PutUint16(buf[utmpRecordSize:utmpRecordSize+2], 1) // not a user process
	binary.LittleEndian.PutUint16(buf[2*utmpRecordSize:2*utmpRecordSize+2], utmpUserProcess)

	n, err := ParseUtmpUsers(buf)
	if err != nil {
		t.Fatalf("ParseUtmpUsers: %v", err)
	}
	if n != 2 {
		t.Errorf("users = %d, want 2", n)
	}
}

func TestParseOSRelease(t *testing.T) {
	b := []byte("NAME=\"Ubuntu\"\nVERSION=\"22.04.3 LTS (Jammy Jellyfish)\"\nID=ubuntu\n")
	if got := ParseOSRelease(b); got != "Ubuntu 22.04.3 LTS (Jammy Jellyfish)" {
		t.Errorf("ParseOSRelease = %q", got)
	}
}

func TestParseCPUInfo(t *testing.T) {
	b := []byte("processor\t: 0\nmodel name\t: Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz\n\n" +
		"processor\t: 1\nmodel name\t: Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz\n")
	info := ParseCPUInfo(b)
	if info.Cores != 2 {
		t.Errorf("cores = %d, want 2", info.Cores)
	}
	if info.Model != "Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz" {
		t.Errorf("model = %q", info.Model)
	}
}
