package controller

import (
	"context"
	"fmt"

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
	if r.KeepalivedConfig.IsPrimary {
		prio = 200
	}

	vips, err := privateVIPs(&lbconfig.Spec.VirtualAddresses, false)
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
		"SrcIP":     internalIPs.myInternalIP(r.KeepalivedConfig.IsPrimary),
		"DstIP":     internalIPs.peerInternalIP(r.KeepalivedConfig.IsPrimary),
		"VIPs":      vips,
	})
}
