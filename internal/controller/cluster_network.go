package controller

import (
	"context"
	"fmt"

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
