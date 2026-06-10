package controller

import (
	"context"
	"fmt"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	lb "github.com/projectsyn/bootc-load-balancer-controller/api/v1alpha1"
)

func (r *LoadBalancerConfigReconciler) RenderHAProxyAPIConfig(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (string, error) {
	l := logf.FromContext(ctx)

	templateData, err := r.haproxyTemplateData(ctx, lbconfig)
	if err != nil {
		return "", err
	}

	l.Info("Rendering API server HAProxy config", "lbconfig", lbconfig.Name)
	return renderTemplate(lbconfig.Spec.Distribution, "haproxy.api.cfg.tmpl", templateData)
}

func (r *LoadBalancerConfigReconciler) RenderHAProxyIngressConfig(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (string, error) {
	l := logf.FromContext(ctx)

	templateData, err := r.haproxyTemplateData(ctx, lbconfig)
	if err != nil {
		return "", err
	}

	l.Info("Rendering ingress HAProxy config", "lbconfig", lbconfig.Name)
	return renderTemplate(lbconfig.Spec.Distribution, "haproxy.ingress.cfg.tmpl", templateData)
}

func (r *LoadBalancerConfigReconciler) haproxyTemplateData(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (map[string]any, error) {
	frontends, err := getHAProxyFrontends(&lbconfig.Spec.VirtualAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare ingress HAProxy frontends: %w", err)
	}

	backends, err := r.getHAProxyBackends(ctx, &lbconfig.Spec.APIBackend.NodeSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare ingress HAProxy backends: %w", err)
	}
	return map[string]any{
		"Frontends": frontends,
		"Backends":  backends,
	}, nil
}
