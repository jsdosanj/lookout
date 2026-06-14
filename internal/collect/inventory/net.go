package inventory

import "net"

// networkInterfaces lists up, non-loopback interfaces with an IPv4 address,
// using the standard library so it behaves identically on every OS.
func networkInterfaces() []NIC {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []NIC
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		n := NIC{Name: ifc.Name, MAC: ifc.HardwareAddr.String()}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				n.IP = ipnet.IP.String()
				break
			}
		}
		out = append(out, n)
	}
	return out
}
