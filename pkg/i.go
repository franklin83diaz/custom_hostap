package pkg

import (
	"fmt"
	"os/exec"
	"strings"
)

// SetNMManagedState configures whether NetworkManager should manage a specific interface.
//
// Parameters:
//   - interfaceName: The name of the card (e.g., "wlan0").
//   - managed: true to enable (normal Wi-Fi), false to disable (ignore card).
func SetNMManagedState(interfaceName string, managed bool) error {
	state := "no"
	if managed {
		state = "yes"
	}

	// We use 'nmcli' device set [iface] managed [yes/no]
	// This command tells the NetworkManager daemon dynamically to stop/start handling this device.
	cmd := exec.Command("nmcli", "device", "set", interfaceName, "managed", state)

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to set managed=%s for %s: %v", state, interfaceName, err)
	}

	return nil
}

// Kill wpa_supplicant for a specific interface
func KillWpaSupplicantForInterface(ifname string) {
	cmd := exec.Command("pgrep", "-f", fmt.Sprintf("wpa_supplicant.*%s", ifname))
	output, err := cmd.Output()

	if err != nil {
		fmt.Printf("  No wpa_supplicant found for %s (OK)\n", ifname)
		return
	}

	pid := strings.TrimSpace(string(output))
	if pid != "" {
		fmt.Printf("  Killing wpa_supplicant (PID: %s) for %s\n", pid, ifname)
		exec.Command("kill", "-9", pid).Run()
	}
}

func SetInterfaceDown(ifname string) error {
	cmd := exec.Command("ip", "link", "set", ifname, "down")
	return cmd.Run()
}

func SetInterfaceUp(ifname string) error {
	cmd := exec.Command("ip", "link", "set", ifname, "up")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}
	return nil
}

// KillHostapdProcesses kills any running hostapd processes for a specific interface
func KillHostapdProcesses(ifname string) {
	// Kill by interface name match
	cmd := exec.Command("pgrep", "-f", fmt.Sprintf("hostapd.*%s", ifname))
	output, err := cmd.Output()

	if err != nil {
		// Also try general hostapd processes
		cmd = exec.Command("pkill", "-9", "hostapd")
		cmd.Run()
		return
	}

	pid := strings.TrimSpace(string(output))
	if pid != "" {
		fmt.Printf("  Killing hostapd (PID: %s) for %s\n", pid, ifname)
		exec.Command("kill", "-9", pid).Run()
	}
}

// KillDnsmasqProcesses kills any running dnsmasq processes for a specific interface
func KillDnsmasqProcesses(ifname string) {
	// Kill by interface name match
	cmd := exec.Command("pgrep", "-f", fmt.Sprintf("dnsmasq.*%s", ifname))
	output, err := cmd.Output()

	if err != nil {
		// Also try general dnsmasq processes
		cmd = exec.Command("pkill", "-9", "dnsmasq")
		cmd.Run()
		return
	}

	pid := strings.TrimSpace(string(output))
	if pid != "" {
		fmt.Printf("  Killing dnsmasq (PID: %s) for %s\n", pid, ifname)
		exec.Command("kill", "-9", pid).Run()
	}
}

// FlushIPAddresses removes all IP addresses from an interface
func FlushIPAddresses(ifname string) error {
	cmd := exec.Command("ip", "addr", "flush", "dev", ifname)
	return cmd.Run()
}

// ResetInterface performs a complete reset of the wireless interface
func ResetInterface(ifname string) error {
	fmt.Printf("Resetting interface %s...\n", ifname)

	// 1. Kill any related processes
	KillHostapdProcesses(ifname)
	KillDnsmasqProcesses(ifname)
	KillWpaSupplicantForInterface(ifname)

	// 2. Disable NetworkManager management
	if err := SetNMManagedState(ifname, false); err != nil {
		fmt.Printf("  Warning: could not disable NetworkManager: %v\n", err)
	}

	// 3. Bring interface down
	if err := SetInterfaceDown(ifname); err != nil {
		fmt.Printf("  Warning: could not bring interface down: %v\n", err)
	}

	// 4. Flush IP addresses
	if err := FlushIPAddresses(ifname); err != nil {
		fmt.Printf("  Warning: could not flush IP addresses: %v\n", err)
	}

	// 5. Unblock rfkill if necessary
	if err := UnblockRFKill(ifname); err != nil {
		fmt.Printf("  Warning: could not unblock rfkill: %v\n", err)
	}

	// 6. Bring interface back up
	if err := SetInterfaceUp(ifname); err != nil {
		return fmt.Errorf("failed to bring interface up: %v", err)
	}

	fmt.Printf("  Interface %s reset complete\n", ifname)
	return nil
}
