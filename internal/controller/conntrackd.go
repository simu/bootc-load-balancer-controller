package controller

import (
	"context"
	"fmt"

	lb "github.com/simu/bootc-load-balancer-controller/api/v1alpha1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *LoadBalancerConfigReconciler) RenderConntrackdConfig(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (string, error) {
	l := logf.FromContext(ctx)

	internalIPs, err := r.internalIPs()
	if err != nil {
		return "", fmt.Errorf("failed to compute internal IPs: %w", err)
	}
	clusterInterface, err := r.ClusterNetworkInterface(ctx)
	if err != nil {
		return "", nil
	}

	l.Info("Rendering conntrackd config", "interface", clusterInterface)
	return renderTemplate("", "conntrackd.conf.tmpl", map[string]any{
		"Interface": clusterInterface,
		"SrcIP":     internalIPs.myInternalIP(r.KeepalivedConfig.IsPrimary),
		"DstIP":     internalIPs.peerInternalIP(r.KeepalivedConfig.IsPrimary),
	})
}
