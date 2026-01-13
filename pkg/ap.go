package pkg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/vishvananda/netlink"
)

// UnblockRFKill attempts to unblock rfkill for the given interface by finding its phy number
func UnblockRFKill(ifaceName string) error {
	// First try to unblock all wifi
	cmd := exec.Command("rfkill", "unblock", "wifi")
	if err := cmd.Run(); err != nil {
		// Non-critical error, just warn
		fmt.Printf("  rfkill unblock wifi warning: %v\n", err)
	}

	// Try to find the specific phy for more targeted unblock
	phyPath := fmt.Sprintf("/sys/class/net/%s/phy80211/name", ifaceName)
	phyData, err := os.ReadFile(phyPath)
	if err != nil {
		// If we can't read phy, that's ok - we already tried unblock wifi
		return nil
	}

	phyName := strings.TrimSpace(string(phyData))

	// Try to unblock specific phy (use index number instead of name)
	phyIndex := ""
	if len(phyName) > 3 && phyName[:3] == "phy" {
		phyIndex = phyName[3:]
	}

	if phyIndex != "" {
		cmd = exec.Command("rfkill", "unblock", phyIndex)
		if err := cmd.Run(); err != nil {
			// Non-critical, we already did wifi unblock
			fmt.Printf("  rfkill unblock %s warning: %v\n", phyIndex, err)
		}
	}

	return nil
}

// WifiStandard represents the WiFi generation to use (up to Wi-Fi 6)
type WifiStandard string

const (
	Wifi4 WifiStandard = "wifi4" // 802.11n - 2.4GHz / 5GHz
	Wifi5 WifiStandard = "wifi5" // 802.11ac - 5GHz
	Wifi6 WifiStandard = "wifi6" // 802.11ax - 2.4GHz / 5GHz (NO 6GHz here)
)

// WifiConfig holds the WiFi configuration parameters
type WifiConfig struct {
	Standard WifiStandard // Wifi4, Wifi5, Wifi6
	Band     string       // "2.4" or "5" (GHz) - optional, auto-selected if empty
	Channel  int          // optional, auto-selected if 0
}

// normalizeBand maps common inputs to "2.4", "5", or "" (auto)
func normalizeBand(b string) string {
	b = strings.TrimSpace(strings.ToLower(b))
	switch b {
	case "", "auto":
		return ""
	case "2.4", "2.4ghz", "24", "2", "2g", "2ghz":
		return "2.4"
	case "5", "5ghz", "5g":
		return "5"
	default:
		// unknown -> treat as auto
		return ""
	}
}

// configureWifiSettings converts WifiConfig into hostapd parameters (up to Wi-Fi 6)
func configureWifiSettings(config *WifiConfig) (hwMode string, channel int, ieee80211n, ieee80211ac, ieee80211ax bool) {
	band := normalizeBand(config.Band)

	// Default band by standard (sane defaults)
	if band == "" {
		switch config.Standard {
		case Wifi5, Wifi6:
			band = "5"
		case Wifi4:
			fallthrough
		default:
			band = "2.4"
		}
	}

	// hw_mode
	if band == "2.4" {
		hwMode = "g"
	} else {
		hwMode = "a"
	}

	// default channel
	channel = config.Channel
	if channel == 0 {
		if band == "2.4" {
			channel = 6
		} else {
			channel = 36
		}
	}

	// enable standards
	switch config.Standard {
	case Wifi4:
		ieee80211n = true
	case Wifi5:
		ieee80211n = true
		ieee80211ac = true
	case Wifi6:
		ieee80211n = true
		ieee80211ac = true
		ieee80211ax = true
	default:
		// default to Wi-Fi 6
		ieee80211n = true
		ieee80211ac = true
		ieee80211ax = true
	}

	// ac only valid on 5GHz
	if band != "5" {
		ieee80211ac = false
	}

	return hwMode, channel, ieee80211n, ieee80211ac, ieee80211ax
}

// StartHostapd configures and starts hostapd.
// addrAndMask example: "192.168.107.1/24"
func StartHostapd(ctx context.Context, ifaceName, addrAndMask, ssid, password string, config *WifiConfig) (*exec.Cmd, error) {
	// Default config if none provided: Wi-Fi 6 on 5GHz
	if config == nil {
		config = &WifiConfig{
			Standard: Wifi6,
			Band:     "5",
		}
	}

	hwMode, channel, ieee80211n, ieee80211ac, ieee80211ax := configureWifiSettings(config)

	// Unblock rfkill - this is usually not critical
	UnblockRFKill(ifaceName)

	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("interface not found %s: %v", ifaceName, err)
	}

	addr, _ := netlink.ParseAddr(addrAndMask)
	if err := netlink.AddrAdd(link, addr); err != nil {
		fmt.Printf("Note about IP: %v (it may have already been assigned)\n", err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("error bringing up the interface: %v", err)
	}

	confContent := fmt.Sprintf(`interface=%s
driver=nl80211
ssid=%s
hw_mode=%s
channel=%d
wpa=2
wpa_passphrase=%s
wpa_key_mgmt=WPA-PSK
rsn_pairwise=CCMP`, ifaceName, ssid, hwMode, channel, password)

	if ieee80211n {
		confContent += "\nieee80211n=1"
	}
	if ieee80211ac {
		confContent += "\nieee80211ac=1"
	}
	if ieee80211ax {
		confContent += "\nieee80211ax=1"
	}

	configFile := "hostapd_temp.conf"
	if err := os.WriteFile(configFile, []byte(confContent), 0644); err != nil {
		return nil, fmt.Errorf("could not create hostapd config file: %v", err)
	}

	cmd := exec.CommandContext(ctx, "hostapd", configFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
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
