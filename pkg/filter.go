package pkg

import (
	"context"
	"fmt"
)

// EnsureDnsmasqFirewall opens ports needed for dnsmasq on lanIface.
// If enableDNS is true, it opens DNS 53/udp and 53/tcp in addition to DHCP 67/udp.
// If enableDNS is false, it removes all firewall rules.
func EnsureDnsmasqFirewall(ctx context.Context, lanIface string, enableDNS bool) error {
	if lanIface == "" {
		return fmt.Errorf("lanIface is required")
	}

	// Remove firewall rules if disabling
	if !enableDNS {
		// Remove DNS rules
		_ = iptablesDeleteIfPresent(ctx,
			[]string{"-C", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
			[]string{"-D", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
		)

		_ = iptablesDeleteIfPresent(ctx,
			[]string{"-C", "INPUT", "-i", lanIface, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
			[]string{"-D", "INPUT", "-i", lanIface, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
		)

		// Remove DHCP rule
		_ = iptablesDeleteIfPresent(ctx,
			[]string{"-C", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "67", "-j", "ACCEPT"},
			[]string{"-D", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "67", "-j", "ACCEPT"},
		)

		return nil
	}

	// DHCPv4 server port (using -I to insert at beginning, before UFW rules)
	if err := iptablesEnsure(ctx,
		[]string{"-C", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "67", "-j", "ACCEPT"},
		[]string{"-I", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "67", "-j", "ACCEPT"},
	); err != nil {
		return fmt.Errorf("failed to allow DHCP (udp/67) on %s: %v", lanIface, err)
	}

	// DNS UDP 53 (using -I to insert at beginning, before UFW rules)
	if err := iptablesEnsure(ctx,
		[]string{"-C", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
		[]string{"-I", "INPUT", "-i", lanIface, "-p", "udp", "--dport", "53", "-j", "ACCEPT"},
	); err != nil {
		return fmt.Errorf("failed to allow DNS (udp/53) on %s: %v", lanIface, err)
	}

	// DNS TCP 53 (using -I to insert at beginning, before UFW rules)
	if err := iptablesEnsure(ctx,
		[]string{"-C", "INPUT", "-i", lanIface, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
		[]string{"-I", "INPUT", "-i", lanIface, "-p", "tcp", "--dport", "53", "-j", "ACCEPT"},
	); err != nil {
		return fmt.Errorf("failed to allow DNS (tcp/53) on %s: %v", lanIface, err)
	}

	return nil
}
