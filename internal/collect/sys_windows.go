//go:build windows

package collect

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ps runs a PowerShell command non-interactively with a fixed script (no user
// input is interpolated, so there is no injection surface).
func ps(script string) (string, error) {
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func collectHost() (Host, error) {
	h := Host{Platform: "windows"}
	h.Hostname, _ = os.Hostname()
	if v, err := ps("(Get-CimInstance Win32_OperatingSystem).Caption"); err == nil {
		h.Version = v
	}
	if v, err := ps("(Get-CimInstance Win32_OperatingSystem).Version"); err == nil {
		h.Kernel = v
	}
	if v, err := ps("[int]((Get-Date) - (Get-CimInstance Win32_OperatingSystem).LastBootUpTime).TotalSeconds"); err == nil {
		h.UptimeSeconds, _ = strconv.ParseInt(v, 10, 64)
	}
	if v, err := ps("$c=Get-CimInstance Win32_ComputerSystem; \"$($c.Manufacturer) $($c.Model)\""); err == nil {
		h.Virtualization = winVirt(v)
	}
	if v, err := ps("(Get-BitLockerVolume -MountPoint $env:SystemDrive).VolumeStatus"); err == nil {
		h.Encryption = parseBitLocker(v)
	}
	return h, nil
}

func collectSpecs() (Specs, error) {
	var s Specs
	if v, err := ps("(Get-CimInstance Win32_Processor | Select-Object -First 1).Name"); err == nil {
		s.CPUModel = v
	}
	if v, err := ps("(Get-CimInstance Win32_ComputerSystem).NumberOfLogicalProcessors"); err == nil {
		s.CPUCores, _ = strconv.Atoi(v)
	}
	if v, err := ps("(Get-CimInstance Win32_Processor | Measure-Object -Property LoadPercentage -Average).Average"); err == nil {
		s.CPUPercent, _ = strconv.ParseFloat(strings.TrimSpace(v), 64)
	}
	if v, err := ps("[math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory/1MB)"); err == nil {
		s.MemTotalMB, _ = strconv.ParseUint(v, 10, 64)
	}
	if v, err := ps("$o=Get-CimInstance Win32_OperatingSystem; [math]::Round(($o.TotalVisibleMemorySize-$o.FreePhysicalMemory)/1KB)"); err == nil {
		s.MemUsedMB, _ = strconv.ParseUint(v, 10, 64)
	}
	if out, err := ps(`Get-CimInstance Win32_LogicalDisk -Filter 'DriveType=3' | ForEach-Object { "$($_.DeviceID)` + "\t" + `$($_.Size)` + "\t" + `$($_.FreeSpace)" }`); err == nil {
		s.Disks = parseWinDisks(out)
	}
	return s, nil
}

func collectPackages() ([]Package, error) {
	const q = `Get-ItemProperty 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*','HKLM:\Software\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*' | Where-Object { $_.DisplayName } | ForEach-Object { "$($_.DisplayName)` + "\t" + `$($_.DisplayVersion)" }`
	out, err := ps(q)
	if err != nil {
		return nil, err
	}
	return parseTabPackages(out), nil
}

func collectServices() ([]Service, error) {
	out, err := ps(`Get-Service | ForEach-Object { "$($_.Name)` + "\t" + `$($_.Status)" }`)
	if err != nil {
		return nil, err
	}
	var svcs []Service
	for _, line := range strings.Split(out, "\n") {
		name, st, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok || name == "" {
			continue
		}
		status := "stopped"
		if strings.EqualFold(st, "Running") {
			status = "running"
		}
		svcs = append(svcs, Service{Name: name, Status: status})
	}
	return svcs, nil
}
