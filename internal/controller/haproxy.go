package controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	lb "github.com/simu/bootc-load-balancer-controller/api/v1alpha1"
)

type BackendList (*[]Backend)

func (r *LoadBalancerConfigReconciler) RenderHAProxyAPIConfig(ctx context.Context, lbconfig *lb.LoadBalancerConfig, localBackends *LocalBackendConfiguration) (string, error) {
	l := logf.FromContext(ctx)

	localAPIBackends := BackendList(nil)
	if localBackends != nil {
		localAPIBackends = &localBackends.API
	}

	templateData, err := r.haproxyTemplateData(ctx, lbconfig, &lbconfig.Spec.APIBackend.NodeSelector, localAPIBackends)
	if err != nil {
		return "", err
	}

	l.Info("Rendering API server HAProxy config", "lbconfig", lbconfig.Name)
	return renderTemplate(lbconfig.Spec.Distribution, "haproxy.api.cfg.tmpl", templateData)
}

func (r *LoadBalancerConfigReconciler) RenderHAProxyIngressConfig(ctx context.Context, lbconfig *lb.LoadBalancerConfig, localBackends *LocalBackendConfiguration) (string, error) {
	l := logf.FromContext(ctx)
	localIngressBackends := BackendList(nil)
	if localBackends != nil {
		localIngressBackends = &localBackends.Ingress
	}

	templateData, err := r.haproxyTemplateData(ctx, lbconfig, &lbconfig.Spec.IngressBackend.NodeSelector, localIngressBackends)
	if err != nil {
		return "", err
	}

	l.Info("Rendering ingress HAProxy config", "lbconfig", lbconfig.Name)
	return renderTemplate(lbconfig.Spec.Distribution, "haproxy.ingress.cfg.tmpl", templateData)
}

func (r *LoadBalancerConfigReconciler) haproxyTemplateData(ctx context.Context, lbconfig *lb.LoadBalancerConfig, ls *metav1.LabelSelector, localBackends *[]Backend) (map[string]any, error) {
	frontends, err := getHAProxyFrontends(&lbconfig.Spec.VirtualAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare ingress HAProxy frontends: %w", err)
	}

	backends := localBackends
	if backends == nil {
		realbackends, err := r.getHAProxyBackends(ctx, ls)
		backends = &realbackends
		if err != nil {
			return nil, fmt.Errorf("failed to prepare ingress HAProxy backends: %w", err)
		}
	}
	return map[string]any{
		"Frontends": frontends,
		"Backends":  backends,
	}, nil
}
