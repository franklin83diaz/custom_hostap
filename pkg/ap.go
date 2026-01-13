package pkg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/vishvananda/netlink"
)

// StartHostapd configures and starts the hostapd service.
// addrAndMask example: "192.168.107.1/24"
func StartHostapd(ctx context.Context, ifaceName, addrAndMask, ssid, password string) (*exec.Cmd, error) {
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("interface not found %s: %v", ifaceName, err)
	}

	addr, _ := netlink.ParseAddr(addrAndMask)

	if err := netlink.AddrAdd(link, addr); err != nil {
		fmt.Printf("Note about IP: %v (it may have already been assigned)\n", err)
	}

	// 3. Bring the interface up (UP)
	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("error bringing up the interface: %v", err)
	}

	// 4. Create temporary configuration file for hostapd
	// This defines the network name (SSID) and password
	confContent := fmt.Sprintf(`interface=%s
driver=nl80211
ssid=%s
hw_mode=a
channel=36
ieee80211n=1
ieee80211ac=1
wpa=2
wpa_passphrase=%s
wpa_key_mgmt=WPA-PSK
rsn_pairwise=CCMP`, ifaceName, ssid, password)

	configFile := "hostapd_temp.conf"
	// In modern Go versions use os.WriteFile, in older ones ioutil.WriteFile
	if err := os.WriteFile(configFile, []byte(confContent), 0644); err != nil {
		return nil, fmt.Errorf("could not create hostapd config file: %v", err)
	}

	// 5. Run hostapd (blocking)
	// Use Cmd to see output in console
	cmd := exec.CommandContext(ctx, "hostapd", configFile)
	// Send hostapd logs to your program output
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Put hostapd in its own process group so we can stop the whole group cleanly.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// StartDnsmasq runs dnsmasq in foreground (--no-daemon) with a /24 DHCP range,
// bound to the given interface and listen IP.
//
// Needs root privileges. The simplest is to run your Go program with sudo.
func StartDnsmasq(ctx context.Context, iface string, listenIP string) (*exec.Cmd, error) {
	if iface == "" || listenIP == "" {
		return nil, fmt.Errorf("iface and listenIP are required")
	}

	// Build DHCP range based on the listen IP, assuming /24:
	// e.g. 192.168.107.1 -> 192.168.107.10 - 192.168.107.200
	var a, b, c, d int
	if _, err := fmt.Sscanf(listenIP, "%d.%d.%d.%d", &a, &b, &c, &d); err != nil ||
		a < 0 || a > 255 || b < 0 || b > 255 || c < 0 || c > 255 || d < 0 || d > 255 {
		return nil, fmt.Errorf("invalid listen IP: %q", listenIP)
	}
	rangeStart := fmt.Sprintf("%d.%d.%d.%d", a, b, c, 10)
	rangeEnd := fmt.Sprintf("%d.%d.%d.%d", a, b, c, 200)

	args := []string{
		"--no-daemon",
		"--conf-file=/dev/null",
		"--interface=" + iface,
		"--bind-interfaces",
		"--listen-address=" + listenIP,
		"--dhcp-range=" + rangeStart + "," + rangeEnd + ",255.255.255.0,12h",
		"--dhcp-option=option:router," + listenIP,
		"--dhcp-option=option:dns-server," + listenIP,
		// If you want DHCP only (no DNS), uncomment:
		// "--port=0",
	}

	cmd := exec.CommandContext(ctx, "dnsmasq", args...)
	// Send dnsmasq logs to your program output
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Put dnsmasq in its own process group so we can stop the whole group cleanly.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func StopCmd(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	// Kill the process group (negative PID) for a clean shutdown.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	_, _ = cmd.Process.Wait()
}
