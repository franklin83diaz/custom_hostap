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

	// Disable NetworkManager management for this interface
	pkg.SetNMManagedState(targetIface.Name, false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmdHostapd, err := pkg.StartHostapd(ctx, targetIface.Name, "192.168.107.1/24", "MySSID", password)
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
