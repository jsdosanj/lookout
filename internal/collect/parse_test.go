package collect

import "testing"

func TestParseOSRelease(t *testing.T) {
	in := `NAME="Ubuntu"
ID=ubuntu
VERSION="22.04.4 LTS (Jammy Jellyfish)"
PRETTY_NAME="Ubuntu 22.04.4 LTS"`
	id, ver := parseOSRelease(in)
	if id != "ubuntu" {
		t.Errorf("id = %q, want ubuntu", id)
	}
	if ver != "Ubuntu 22.04.4 LTS" {
		t.Errorf("version = %q, want PRETTY_NAME", ver)
	}
}

func TestParseMeminfo(t *testing.T) {
	in := "MemTotal:        4096000 kB\nMemFree:         1000000 kB\nMemAvailable:    2048000 kB\n"
	total, used := parseMeminfo(in)
	if total != 4000 { // 4096000 kB / 1024
		t.Errorf("total = %d MB, want 4000", total)
	}
	if used != 2000 { // (4096000-2048000)/1024
		t.Errorf("used = %d MB, want 2000", used)
	}
}

func TestParseCPUInfo(t *testing.T) {
	in := "processor\t: 0\nmodel name\t: Intel Xeon\nprocessor\t: 1\nmodel name\t: Intel Xeon\n"
	model, cores := parseCPUInfo(in)
	if model != "Intel Xeon" {
		t.Errorf("model = %q", model)
	}
	if cores != 2 {
		t.Errorf("cores = %d, want 2", cores)
	}
}

func TestParseUptime(t *testing.T) {
	if got := parseUptime("12345.67 8910.11"); got != 12345 {
		t.Errorf("uptime = %d, want 12345", got)
	}
}

func TestParseLoadAvg(t *testing.T) {
	got := parseLoadAvg("0.50 0.40 0.30 1/200 1234")
	if len(got) != 3 || got[0] != 0.50 || got[2] != 0.30 {
		t.Errorf("loadavg = %v", got)
	}
	if parseLoadAvg("only one") != nil {
		t.Error("expected nil for malformed loadavg")
	}
}

func TestParseDf(t *testing.T) {
	in := `Filesystem 1024-blocks    Used Available Capacity Mounted on
/dev/disk1s1   976490576 8000000 900000000      48% /
devfs                  0       0         0     100% /dev
map auto_home          0       0         0     100% /System/Volumes/Data/home`
	disks := parseDf(in)
	if len(disks) != 1 {
		t.Fatalf("got %d disks, want 1 (devfs/0-size filtered)", len(disks))
	}
	if disks[0].Mount != "/" || disks[0].TotalMB == 0 {
		t.Errorf("disk = %+v", disks[0])
	}
}

func TestParseTabPackages(t *testing.T) {
	pkgs := parseTabPackages("nginx\t1.24.0\nopenssl\t3.0.2\n\nbad-line-no-tab\n")
	if len(pkgs) != 2 {
		t.Fatalf("got %d packages, want 2", len(pkgs))
	}
	if pkgs[0].Name != "nginx" || pkgs[0].Version != "1.24.0" {
		t.Errorf("pkg = %+v", pkgs[0])
	}
}

func TestParseSystemctl(t *testing.T) {
	in := "ssh.service     loaded active running OpenBSD Secure Shell\ncron.service    loaded active exited  Regular background program\n"
	svcs := parseSystemctl(in)
	if len(svcs) != 2 {
		t.Fatalf("got %d services, want 2", len(svcs))
	}
	if svcs[0].Name != "ssh" || svcs[0].Status != "running" {
		t.Errorf("svc0 = %+v", svcs[0])
	}
	if svcs[1].Status != "stopped" {
		t.Errorf("svc1 status = %q, want stopped", svcs[1].Status)
	}
}

func TestParseLaunchctl(t *testing.T) {
	in := "PID\tStatus\tLabel\n123\t0\tcom.apple.foo\n-\t0\tcom.apple.bar\n"
	svcs := parseLaunchctl(in)
	if len(svcs) != 2 {
		t.Fatalf("got %d services, want 2", len(svcs))
	}
	if svcs[0].Status != "running" || svcs[1].Status != "stopped" {
		t.Errorf("statuses = %q, %q", svcs[0].Status, svcs[1].Status)
	}
}

func TestParseVMStat(t *testing.T) {
	in := "Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages active:                            65536.\nPages wired down:                        65536.\nPages occupied by compressor:                0.\n"
	// (65536 + 65536) pages * 16384 bytes = 2 GiB used = 2048 MB
	if got := parseVMStat(in, 16384); got != 2048 {
		t.Errorf("used = %d MB, want 2048", got)
	}
}

func TestParseWinDisks(t *testing.T) {
	in := "C:\t107374182400\t53687091200\nD:\t0\t0\n"
	disks := parseWinDisks(in)
	if len(disks) != 1 {
		t.Fatalf("got %d disks, want 1 (0-size filtered)", len(disks))
	}
	if disks[0].Mount != "C:" || disks[0].TotalMB != 102400 || disks[0].UsedMB != 51200 {
		t.Errorf("disk = %+v", disks[0])
	}
}
