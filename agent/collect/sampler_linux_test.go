//go:build linux

package collect

import "testing"

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
