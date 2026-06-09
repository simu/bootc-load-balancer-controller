package controller

import (
	"context"
	"fmt"
	"net"

	lb "github.com/projectsyn/bootc-load-balancer-controller/api/v1alpha1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *LoadBalancerConfigReconciler) RenderKeepalivedDummyNMConnection(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (string, error) {
	l := logf.FromContext(ctx)

	vips, err := publicVIPs(&lbconfig.Spec.VirtualAddresses)
	if err != nil {
		return "", fmt.Errorf("failed to prepare public IPv4 VIPs: %w", err)
	}
	vips4 := []string{}
	vips6 := []string{}

	for _, vip := range vips {
		if vip.Addr().Is4() {
			vips4 = append(vips4, vip.String())
		} else {
			vips6 = append(vips6, vip.String())
		}
	}

	l.Info("Rendering NetworkManager keepalived dummy interface config")
	return renderTemplate("", "keepalived.nmconnection.tmpl", map[string]any{
		"VIPs4": vips4,
		"VIPs6": vips6,
	})
}

func (r *LoadBalancerConfigReconciler) RenderPublicNMConnection(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (string, error) {
	l := logf.FromContext(ctx)

	macAddr := ""
	ifList, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("fetching interfaces: %w", err)
	}
	for _, iface := range ifList {
		if iface.Name == r.PublicInterface {
			macAddr = iface.HardwareAddr.String()
		}
	}

	l.Info("Rendering NetworkManager public interface config", "interface", r.PublicInterface)
	return renderTemplate("", "public.nmconnection.tmpl", map[string]any{
		"MACAddress": macAddr,
	})
}
