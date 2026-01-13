package main

import (
	"context"
	"fmt"
	"log"
	"os"
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

	targetIface := wInterfaces[0]
	fmt.Printf("\nTarget: %s\n", targetIface.Name)

	// Disable NetworkManager management for this interface
	pkg.SetNMManagedState(targetIface.Name, false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_, err := pkg.StartHostapd(ctx, targetIface.Name, "192.168.107.1/24", "MySSID", "MyPassword")
		if err != nil {
			log.Fatalf("Error starting AP: %v", err)
		}
	}()

	iface := targetIface.Name
	ip := "192.168.107.1"

	cmd, err := pkg.StartDnsmasq(ctx, iface, ip)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start dnsmasq:", err)
		os.Exit(1)
	}

	// Wait until dnsmasq exits or context is canceled
	err = cmd.Wait()
	// If context canceled, ensure it stops
	pkg.StopCmd(cmd)

	if err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "dnsmasq exited:", err)
	}
}
