package controller

import (
	"context"
	"fmt"

	lb "github.com/projectsyn/bootc-load-balancer-controller/api/v1alpha1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *LoadBalancerConfigReconciler) RenderConntrackdConfig(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (string, error) {
	l := logf.FromContext(ctx)

	internalIPs, err := r.internalIPs(lbconfig)
	if err != nil {
		return "", fmt.Errorf("failed to compute internal IPs: %w", err)
	}

	l.Info("Rendering conntrackd config", "interface", r.KeepalivedConfig.Interface)
	return renderTemplate(lbconfig.Spec.Distribution, "conntrackd.conf.tmpl", map[string]any{
		"Interface": r.KeepalivedConfig.Interface,
		"SrcIP":     internalIPs.myInternalIP(r.KeepalivedConfig.IsPrimary, false),
		"DstIP":     internalIPs.peerInternalIP(r.KeepalivedConfig.IsPrimary, false),
	})
}
