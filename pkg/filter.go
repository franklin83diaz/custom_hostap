package pkg

import (
	"context"
	"fmt"
)

// EnsureDnsmasqFirewall opens ports needed for dnsmasq on lanIface.
// If enableDNS is true, it opens DNS 53/udp and 53/tcp in addition to DHCP 67/udp.
func EnsureDnsmasqFirewall(ctx context.Context, lanIface string, enableDNS bool) error {
	if lanIface == "" {
		return fmt.Errorf("lanIface is required")
	}

	// DHCPv4 server port
	if err := iptablesEnsure(ctx,
		[]string{"-C", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "67", "-j", "ACCEPT"},
		[]string{"-A", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "67", "-j", "ACCEPT"},
	); err != nil {
		return fmt.Errorf("failed to allow DHCP (udp/67) on %s: %v", lanIface, err)
	}

	if enableDNS {
		// DNS UDP 53
		if err := iptablesEnsure(ctx,
			[]string{"-C", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
			[]string{"-A", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
		); err != nil {
			return fmt.Errorf("failed to allow DNS (udp/53) on %s: %v", lanIface, err)
		}

		// DNS TCP 53 (algunos clientes/escenarios lo usan)
		if err := iptablesEnsure(ctx,
			[]string{"-C", "INPUT", "-i", lanIface, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
			[]string{"-A", "INPUT", "-i", lanIface, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
		); err != nil {
			return fmt.Errorf("failed to allow DNS (tcp/53) on %s: %v", lanIface, err)
		}
	}

	return nil
}
