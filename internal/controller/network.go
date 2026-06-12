package controller

import (
	"context"
	"fmt"
	"net"

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
	macAddress := ""
	ifList, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("fetching interfaces: %w", err)
	}
	for _, iface := range ifList {
		if iface.Name == interfaceName {
			macAddress = iface.HardwareAddr.String()
		}
	}
	if macAddress == "" {
		return "", fmt.Errorf("failed to find MAC address for interface '%s'", interfaceName)
	}
	return macAddress, nil
}
