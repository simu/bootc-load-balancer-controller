package controller

import (
	"context"
	"fmt"
	"net/netip"

	lb "github.com/simu/bootc-load-balancer-controller/api/v1alpha1"
)

func (r *LoadBalancerConfigReconciler) RenderFirewallDirectRules(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (string, error) {

	natVip, err := netip.ParsePrefix(lbconfig.Spec.VirtualAddresses.NAT.Address)
	if err != nil {
		return "", fmt.Errorf("failed to parse NAT VIP: %w", err)
	}

	return renderTemplate("firewalld", "direct.xml.tmpl", map[string]any{
		"PublicInterface": r.PublicInterface,
		"ClusterNetwork":  r.ClusterNetwork,
		"NATAddress":      natVip.Addr(),
	})
}

func (r *LoadBalancerConfigReconciler) RenderFirewallExternalZone(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (string, error) {
	vips, err := publicVIPs(&lbconfig.Spec.VirtualAddresses)
	if err != nil {
		return "", fmt.Errorf("failed to prepare VIPs: %w", err)
	}

	vips4 := []netip.Prefix{}
	vips6 := []netip.Prefix{}
	for _, vip := range vips {
		if vip.Addr().Is4() {
			vips4 = append(vips4, vip)
		} else {
			vips6 = append(vips6, vip)
		}
	}

	return renderTemplate("firewalld", "zone-external.xml.tmpl", map[string]any{
		"IPv4": vips4,
		"IPv6": vips6,
	})
}
