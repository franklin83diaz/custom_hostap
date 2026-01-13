package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
	"wifigo/pkg"

	"github.com/mdlayher/wifi"
)

type WInterface struct {
	Name string
}

func GetWifi() []WInterface {
	c, err := wifi.New()
	if err != nil {
		log.Fatalf("error open connection wifi: %v", err)
	}
	defer c.Close()

	ifaces, err := c.Interfaces()
	if err != nil {
		log.Fatalf("error listing interfaces: %v", err)
	}

	wInterfaces := []WInterface{}
	for _, ifi := range ifaces {
		if ifi.Name == "" {
			continue
		}
		fmt.Printf("found: %s (ID: %d, PHY: %d, MAC: %s)\n", ifi.Name, ifi.Index, ifi.PHY, ifi.HardwareAddr)
		wInterfaces = append(wInterfaces, WInterface{
			Name: ifi.Name,
		})
	}
	return wInterfaces
}

func main() {

	wInterfaces := GetWifi()
	if len(wInterfaces) == 0 {
		log.Fatal("No wifi interfaces found")
	}

	password := "MyPassword"
	if len(os.Args) > 1 {
		password = os.Args[1]
	}

	targetIface := wInterfaces[0]
	fmt.Printf("\nTarget: %s\n", targetIface.Name)

	// Reset interface completely (kills old processes, cleans IP, disables NM)
	if err := pkg.ResetInterface(targetIface.Name); err != nil {
		log.Fatalf("Failed to reset interface: %v", err)
	}

	// Wait a moment for interface to stabilize
	time.Sleep(1 * time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Configure WiFi settings - Simply select the WiFi generation!
	wifiConfig := &pkg.WifiConfig{
		Standard: pkg.Wifi5, // Options: Wifi4, Wifi5, Wifi6, Wifi7
		// Band and Channel are optional - auto-configured if not specified
		Band: "2.4", // Optional: "2.4" or "5" GHz
		// Channel: 36,       // Optional: auto-selected if 0 or omitted
	}

	cmdHostapd, err := pkg.StartHostapd(ctx, targetIface.Name, "192.168.107.1/24", "MySSID", password, wifiConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start hostapd:", err)
		os.Exit(1)
	}

	// wait 2 seconds
	time.Sleep(2 * time.Second)

	iface := targetIface.Name
	ip := "192.168.107.1"

	cmdDnsmasq, err := pkg.StartDnsmasq(ctx, iface, ip)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start dnsmasq:", err)
		os.Exit(1)
	}

	// Wait until dnsmasq  or Hostapd exits or context is canceled
	errChan := make(chan error, 2)
	go func() {
		errChan <- cmdHostapd.Wait()
	}()
	go func() {
		errChan <- cmdDnsmasq.Wait()
	}()

	// wait 2 seconds
	time.Sleep(2 * time.Second)

	go func() {
		fmt.Println("Press ENTER to stop...")
		fmt.Scanln()
		cancel()
	}()

	wanIface, err := pkg.DetectWANInterface()
	if err != nil {
		fmt.Fprintln(os.Stderr, "DetectWANInterface:", err)
		os.Exit(1)
	}

	_ = pkg.EnableNAT(ctx, "192.168.107.0/24", "wlan0", wanIface)

	select {
	case err = <-errChan:
		// One of the commands exited
		fmt.Println("dnsmasq  or Hostapd exits")
	case <-ctx.Done():
		fmt.Println("Canceled")
		// Context canceled
	}

	// Cleanup
	fmt.Println("Cleaning up...")
	pkg.StopCmd(cmdDnsmasq)
	pkg.StopCmd(cmdHostapd)

	if err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "exited:", err)
	}
}
