package controller

import (
	"context"
	"fmt"
	"net/netip"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	lb "github.com/projectsyn/bootc-load-balancer-controller/api/v1alpha1"
)

type KeepalivedConfig struct {
	Interface string
	IsPrimary bool
}

func (r *LoadBalancerConfigReconciler) RenderKeepalivedConfig(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (string, error) {
	l := logf.FromContext(ctx)

	internalIPs, err := r.internalIPs(lbconfig)
	if err != nil {
		return "", fmt.Errorf("failed to compute internal IPs: %w", err)
	}

	prio := 100
	srcIP := internalIPs.Secondary.String()
	dstIP := internalIPs.Primary.String()
	if r.KeepalivedConfig.IsPrimary {
		prio = 200
		srcIP = internalIPs.Primary.String()
		dstIP = internalIPs.Secondary.String()
	}

	vips, err := keepalivedVIPs(&lbconfig.Spec.VirtualAddresses)
	if err != nil {
		return "", fmt.Errorf("failed to prepare Keepalived VIPs: %w", err)
	}

	if lbconfig.Spec.VirtualAddresses.NAT != nil {
		vips = append(vips, internalIPs.DefaultGateway.String())
	}

	l.Info("Rendering keepalived config", "interface", r.KeepalivedConfig.Interface, "priority", prio)
	return renderTemplate(lbconfig.Spec.Distribution, "keepalived.conf.tmpl", map[string]any{
		"Interface": r.KeepalivedConfig.Interface,
		"Priority":  prio,
		"SrcIP":     srcIP,
		"DstIP":     dstIP,
		"VIPs":      vips,
	})
}

type InternalIPs struct {
	DefaultGateway netip.Addr
	Primary        netip.Addr
	Secondary      netip.Addr
}

func (r *LoadBalancerConfigReconciler) internalIPs(lbconfig *lb.LoadBalancerConfig) (*InternalIPs, error) {
	clusternet, err := netip.ParsePrefix(lbconfig.Spec.ClusterNetwork)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cluster network: %w", err)
	}
	netaddr := clusternet.Masked().Addr()
	defaultGateway := netaddr.Next()
	primaryIP := defaultGateway.Next()
	secondaryIP := primaryIP.Next()
	return &InternalIPs{
		DefaultGateway: defaultGateway,
		Primary:        primaryIP,
		Secondary:      secondaryIP,
	}, nil
}
