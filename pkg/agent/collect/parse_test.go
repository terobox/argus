package collect

import (
	"testing"
)

func TestParseCPUStat(t *testing.T) {
	fixture := []byte(`cpu  100 10 20 1000 30 5 6 7 0 0
cpu0 100 10 20 1000 30 5 6 7 0 0
intr 3123 0 0
`)
	got, err := ParseCPUStat(fixture)
	if err != nil {
		t.Fatalf("ParseCPUStat: %v", err)
	}
	if got.User != 100 || got.Nice != 10 || got.System != 20 || got.Idle != 1000 {
		t.Errorf("base fields = %+v", got)
	}
	if got.Iowait != 30 || got.Irq != 5 || got.Softirq != 6 || got.Steal != 7 {
		t.Errorf("optional fields = %+v", got)
	}
	if got.Total() != 1178 {
		t.Errorf("Total() = %d, want 1178", got.Total())
	}
	if got.IdleTotal() != 1030 {
		t.Errorf("IdleTotal() = %d, want 1030", got.IdleTotal())
	}
}

func TestParseCPUStatMissingLine(t *testing.T) {
	if _, err := ParseCPUStat([]byte("intr 1 2 3\n")); err == nil {
		t.Fatal("expected error for missing aggregate line, got nil")
	}
}

func TestParseMeminfo(t *testing.T) {
	fixture := []byte(`MemTotal:       16384000 kB
MemFree:         8123456 kB
MemAvailable:   12233445 kB
Buffers:          123456 kB
SwapTotal:       4194304 kB
SwapFree:        3145728 kB
`)
	got, err := ParseMeminfo(fixture)
	if err != nil {
		t.Fatalf("ParseMeminfo: %v", err)
	}
	if got.MemTotal != 16384000 || got.MemAvailable != 12233445 {
		t.Errorf("mem = %+v", got)
	}
	if got.SwapTotal != 4194304 || got.SwapFree != 3145728 {
		t.Errorf("swap = %+v", got)
	}
}

func TestParseMeminfoNoSwap(t *testing.T) {
	fixture := []byte("MemTotal:       16384000 kB\nMemAvailable:   12233445 kB\n")
	got, err := ParseMeminfo(fixture)
	if err != nil {
		t.Fatalf("ParseMeminfo: %v", err)
	}
	if got.SwapTotal != 0 || got.SwapFree != 0 {
		t.Errorf("expected zero swap on swap-less host, got %+v", got)
	}
}

func TestParseMeminfoMissingTotal(t *testing.T) {
	if _, err := ParseMeminfo([]byte("MemFree: 100 kB\n")); err == nil {
		t.Fatal("expected error for missing MemTotal, got nil")
	}
}

func TestParseLoadavg(t *testing.T) {
	got, err := ParseLoadavg([]byte("0.52 0.58 0.59 1/912 34567\n"))
	if err != nil {
		t.Fatalf("ParseLoadavg: %v", err)
	}
	if got.Load1 != 0.52 || got.Load5 != 0.58 || got.Load15 != 0.59 {
		t.Errorf("load = %+v", got)
	}
}

func TestParseUptimeSeconds(t *testing.T) {
	got, err := ParseUptimeSeconds([]byte("123456.78 987654.32\n"))
	if err != nil {
		t.Fatalf("ParseUptimeSeconds: %v", err)
	}
	if got != 123456.78 {
		t.Errorf("uptime = %v, want 123456.78", got)
	}
}

func TestParseNetDev(t *testing.T) {
	fixture := []byte(`Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 12345678    1234    0    0    0     0          0         0 12345678    1234    0    0    0     0       0          0
  eth0: 9876543210    8765    0    0    0     0          0         0 4321098765    6543    0    0    0     0       0          0
`)
	got, err := ParseNetDev(fixture)
	if err != nil {
		t.Fatalf("ParseNetDev: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d interfaces, want 2", len(got))
	}
	eth0 := got["eth0"]
	if eth0.RxBytes != 9876543210 || eth0.TxBytes != 4321098765 {
		t.Errorf("eth0 = %+v", eth0)
	}
	if got["lo"].RxBytes != 12345678 || got["lo"].TxBytes != 12345678 {
		t.Errorf("lo = %+v", got["lo"])
	}
}

func TestParseDiskstats(t *testing.T) {
	fixture := []byte(`   8       0 sda 12345 678 987654 12345 2345 567 345678 2345 0 12345 2345 0 0 0 0
   8       1 sda1 111 222 333 444 555 666 777 888 999 0 0 0 0 0 0 0
`)
	got, err := ParseDiskstats(fixture)
	if err != nil {
		t.Fatalf("ParseDiskstats: %v", err)
	}
	sda := got["sda"]
	if sda.ReadBytes != 987654*512 || sda.WriteBytes != 345678*512 {
		t.Errorf("sda = %+v", sda)
	}
}
