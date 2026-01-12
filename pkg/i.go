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
	return cmd.Run()
}
