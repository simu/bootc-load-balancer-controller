package controller

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"tailscale.com/net/routetable"
)

func (r *LoadBalancerConfigReconciler) ClusterNetworkInterface(ctx context.Context) (string, error) {
	l := logf.FromContext(ctx)
	// TODO(sg): how to set limit here?
	routes, err := routetable.Get(200)
	if err != nil {
		return "", fmt.Errorf("failed to list system routes: %w", err)
	}
	clusterIface := ""
	for _, rt := range routes {
		if rt.Dst.Compare(r.ClusterNetwork) == 0 {
			clusterIface = rt.Interface
			l.Info("Found cluster network route", "interface", clusterIface)
			break
		}
	}
	if clusterIface == "" {
		return "", fmt.Errorf("failed to find interface for cluster network '%s'", r.ClusterNetwork)
	}
	return clusterIface, nil
}

func (r *LoadBalancerConfigReconciler) RenderSysctlConf(ctx context.Context) (string, error) {
	l := logf.FromContext(ctx)
	l.Info("Setting sysctl net.ipv6.conf.<interface>.accept_ra=2 on public interface", "interface", r.PublicInterface)
	return renderTemplate("", "sysctl.conf.tmpl", map[string]any{
		"Interface": r.PublicInterface,
	})
}

func macAddressForInterface(interfaceName string) (string, error) {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return "", fmt.Errorf("Unable to find interface: %w", err)
	}
	return iface.HardwareAddr.String(), nil
}

func primaryIPAddressForInterface(interfaceName string, ifaceNet *netip.Prefix) (string, error) {
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return "", fmt.Errorf("Unable to find interface: %w", err)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("Failed to get addresses for interface: %w", err)
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("Interface has no addresses")
	}

	primaryIP := ""
	for _, addr := range addrs {
		ip, err := netip.ParsePrefix(addr.String())
		if ip.Addr().Is6() {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("Failed to parse IP: %w", err)
		}
		if ifaceNet != nil && ip.Bits() == ifaceNet.Bits() {
			primaryIP = ip.String()
			break
		} else if ifaceNet == nil {
			primaryIP = ip.String()
			break
		}
	}
	if primaryIP == "" {
		return "", fmt.Errorf("Didn't find an IP which matches interface network prefix length")
	}

	return primaryIP, nil
}
