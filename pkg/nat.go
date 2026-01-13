package pkg

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	"github.com/vishvananda/netlink"
)

// EnableNAT enables IPv4 forwarding and sets up iptables NAT for lanCIDR (e.g. "192.168.107.0/24").
// lanIface: interface where your clients are (e.g. wlan0 with hostapd)
// wanIface: uplink interface (e.g. eth0)
func EnableNAT(ctx context.Context, lanCIDR, lanIface, wanIface string) error {
	if lanCIDR == "" || lanIface == "" || wanIface == "" {
		return fmt.Errorf("lanCIDR, lanIface and wanIface are required")
	}

	// Validate CIDR
	if _, _, err := net.ParseCIDR(lanCIDR); err != nil {
		return fmt.Errorf("invalid lanCIDR %q: %v", lanCIDR, err)
	}

	// 1) Enable IPv4 forwarding (router mode)
	if err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0644); err != nil {
		return fmt.Errorf("failed to enable ip_forward: %v", err)
	}

	// 2) NAT: MASQUERADE lanCIDR out of wanIface
	//    iptables -t nat -A POSTROUTING -s <lanCIDR> -o <wanIface> -j MASQUERADE
	if err := iptablesEnsure(ctx,
		[]string{"-t", "nat", "-C", "POSTROUTING", "-s", lanCIDR, "-o", wanIface, "-j", "MASQUERADE"},
		[]string{"-t", "nat", "-A", "POSTROUTING", "-s", lanCIDR, "-o", wanIface, "-j", "MASQUERADE"},
	); err != nil {
		return fmt.Errorf("failed to ensure NAT MASQUERADE: %v", err)
	}

	// 3) Allow forwarding LAN -> WAN (new connections)
	//    iptables -A FORWARD -i <lanIface> -o <wanIface> -s <lanCIDR> -j ACCEPT
	if err := iptablesEnsure(ctx,
		[]string{"-C", "FORWARD", "-i", lanIface, "-o", wanIface, "-s", lanCIDR, "-j", "ACCEPT"},
		[]string{"-A", "FORWARD", "-i", lanIface, "-o", wanIface, "-s", lanCIDR, "-j", "ACCEPT"},
	); err != nil {
		return fmt.Errorf("failed to ensure FORWARD LAN->WAN: %v", err)
	}

	// 4) Allow forwarding WAN -> LAN for established/related
	//    iptables -A FORWARD -i <wanIface> -o <lanIface> -d <lanCIDR> -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
	if err := iptablesEnsure(ctx,
		[]string{"-C", "FORWARD", "-i", wanIface, "-o", lanIface, "-d", lanCIDR, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		[]string{"-A", "FORWARD", "-i", wanIface, "-o", lanIface, "-d", lanCIDR, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
	); err != nil {
		return fmt.Errorf("failed to ensure FORWARD WAN->LAN established: %v", err)
	}

	return nil
}

// DisableNAT removes the same rules (useful for cleanup).
func DisableNAT(ctx context.Context, lanCIDR, lanIface, wanIface string) error {
	if _, _, err := net.ParseCIDR(lanCIDR); err != nil {
		return fmt.Errorf("invalid lanCIDR %q: %v", lanCIDR, err)
	}

	// Remove in reverse-ish order; ignore "not found" errors by doing -C before -D
	_ = iptablesDeleteIfPresent(ctx,
		[]string{"-t", "nat", "-C", "POSTROUTING", "-s", lanCIDR, "-o", wanIface, "-j", "MASQUERADE"},
		[]string{"-t", "nat", "-D", "POSTROUTING", "-s", lanCIDR, "-o", wanIface, "-j", "MASQUERADE"},
	)

	_ = iptablesDeleteIfPresent(ctx,
		[]string{"-C", "FORWARD", "-i", lanIface, "-o", wanIface, "-s", lanCIDR, "-j", "ACCEPT"},
		[]string{"-D", "FORWARD", "-i", lanIface, "-o", wanIface, "-s", lanCIDR, "-j", "ACCEPT"},
	)

	_ = iptablesDeleteIfPresent(ctx,
		[]string{"-C", "FORWARD", "-i", wanIface, "-o", lanIface, "-d", lanCIDR, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		[]string{"-D", "FORWARD", "-i", wanIface, "-o", lanIface, "-d", lanCIDR, "-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
	)

	return nil
}

func iptablesEnsure(ctx context.Context, checkArgs, addArgs []string) error {
	// If rule exists, ok
	if err := runCmd(ctx, "iptables", checkArgs...); err == nil {
		return nil
	}
	// Otherwise add it
	if err := runCmd(ctx, "iptables", addArgs...); err != nil {
		return err
	}
	return nil
}

func iptablesDeleteIfPresent(ctx context.Context, checkArgs, delArgs []string) error {
	if err := runCmd(ctx, "iptables", checkArgs...); err != nil {
		return nil // not present
	}
	return runCmd(ctx, "iptables", delArgs...)
}

func runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s failed: %v; output=%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func isDefaultV4Route(r netlink.Route) bool {
	// Caso 1: algunos kernels/devuelven default con Dst == nil
	if r.Dst == nil {
		return true
	}
	// Caso 2: otros devuelven 0.0.0.0/0 explícito
	ip := r.Dst.IP.To4()
	if ip == nil {
		return false
	}
	ones, bits := r.Dst.Mask.Size()
	if bits != 32 {
		return false
	}
	return ip.Equal(net.IPv4(0, 0, 0, 0)) && ones == 0
}

// DetectWANInterface retorna la interfaz usada por la ruta por defecto IPv4.
func DetectWANInterface() (string, error) {
	routes, err := netlink.RouteListFiltered(
		netlink.FAMILY_V4,
		&netlink.Route{Table: 254}, // main
		netlink.RT_FILTER_TABLE,
	)
	if err != nil {
		return "", fmt.Errorf("RouteListFiltered failed: %v", err)
	}

	var best *netlink.Route
	for i := range routes {
		r := routes[i]

		// netlink imprime "Ifindex", pero es el mismo campo (LinkIndex) internamente
		if r.LinkIndex <= 0 {
			continue
		}
		if !isDefaultV4Route(r) {
			continue
		}

		// Si hay varias default routes, escoger por métrica (Priority menor)
		if best == nil {
			best = &r
			continue
		}
		// Priority puede venir 0; en ese caso, tratamos 0 como "sin métrica"
		if r.Priority != 0 && (best.Priority == 0 || r.Priority < best.Priority) {
			best = &r
		}
	}

	if best == nil {
		return "", fmt.Errorf("no default IPv4 route found")
	}

	link, err := netlink.LinkByIndex(best.LinkIndex)
	if err != nil {
		return "", fmt.Errorf("LinkByIndex(%d) failed: %v", best.LinkIndex, err)
	}
	return link.Attrs().Name, nil
}
