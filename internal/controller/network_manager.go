package controller

import (
	"context"
	"fmt"
	"net/netip"

	lb "github.com/projectsyn/bootc-load-balancer-controller/api/v1alpha1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *LoadBalancerConfigReconciler) RenderKeepalivedDummyNMConnection(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (string, error) {
	l := logf.FromContext(ctx)

	vips, err := publicVIPs[netip.Prefix](&lbconfig.Spec.VirtualAddresses, false)
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
