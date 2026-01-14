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

	sSID := "MySSID"
	if len(os.Args) > 1 {
		sSID = os.Args[1]
	}

	password := "MyPassword"
	if len(os.Args) > 2 {
		password = os.Args[2]
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
		Standard: pkg.Wifi6, // Options: Wifi4, Wifi5, Wifi6, Wifi7
		// Band and Channel are optional - auto-configured if not specified
		Band: "5", // Optional: "2.4" or "5" GHz
		// Channel: 36,       // Optional: auto-selected if 0 or omitted
	}

	cmdHostapd, err := pkg.StartHostapd(ctx, targetIface.Name, "192.168.107.1/24", sSID, password, wifiConfig)
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

	_ = pkg.EnsureDnsmasqFirewall(ctx, iface, true)

	_ = pkg.EnableNAT(ctx, "192.168.107.0/24")

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

	// Stop services
	pkg.StopCmd(cmdDnsmasq)
	pkg.StopCmd(cmdHostapd)

	// Remove NAT rules
	fmt.Println("Removing NAT rules...")
	if err := pkg.DisableNAT(context.Background(), "192.168.107.0/24"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove NAT rules: %v\n", err)
	}

	// Remove dnsmasq firewall rules
	fmt.Println("Removing firewall rules...")
	if err := pkg.EnsureDnsmasqFirewall(context.Background(), iface, false); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove firewall rules: %v\n", err)
	}

	// Re-enable Network Manager management
	fmt.Println("Re-enabling Network Manager...")
	if err := pkg.SetNMManagedState(iface, true); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to re-enable Network Manager: %v\n", err)
	}

	if err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "exited:", err)
	}
}
