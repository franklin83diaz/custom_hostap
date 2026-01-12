package pkg

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/vishvananda/netlink"
)

// StartAP c
// addrAndMask example: "192.168.107.1/24"
func StartAP(ifaceName, addrAndMask, ssid, password string) error {
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return fmt.Errorf("interface not found %s: %v", ifaceName, err)
	}

	addr, _ := netlink.ParseAddr(addrAndMask)

	if err := netlink.AddrAdd(link, addr); err != nil {
		fmt.Printf("Note about IP: %v (it may have already been assigned)\n", err)
	}

	// 3. Bring the interface up (UP)
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("error bringing up the interface: %v", err)
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
		return fmt.Errorf("could not create hostapd config file: %v", err)
	}

	fmt.Printf("Starting Hostapd for SSID: %s (IP: %s)...\n", ssid, addrAndMask)

	// 5. Run hostapd (blocking)
	// Use Cmd to see output in console
	cmd := exec.Command("hostapd", configFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hostapd execution error: %v", err)
	}

	return nil
}
