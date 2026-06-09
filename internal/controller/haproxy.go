package controller

import (
	"context"
	"fmt"
	"net/netip"

	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	return renderTemplate(lbconfig.Spec.Distribution, "haproxy.ingress.cfg.tmpl", templateData)
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
	frontends, err := getFrontends(&lbconfig.Spec.VirtualAddresses)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare ingress HAProxy frontends: %w", err)
	}

	backends, err := r.getBackends(ctx, &lbconfig.Spec.APIBackend.NodeSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare ingress HAProxy backends: %w", err)
	}
	return map[string]any{
		"Frontends": frontends,
		"Backends":  backends,
	}, nil
}

type Backend struct {
	Name    string
	Address string
}

func (r *LoadBalancerConfigReconciler) getBackends(ctx context.Context, ls *metav1.LabelSelector) ([]Backend, error) {
	nodeSelector, err := metav1.LabelSelectorAsSelector(ls)
	if err != nil {
		return nil, fmt.Errorf("failed to convert node selector: %w", err)
	}

	var nodes corev1.NodeList
	if err := r.List(ctx, &nodes, client.MatchingLabelsSelector{
		Selector: nodeSelector,
	}); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	backends := []Backend{}
	for _, node := range nodes.Items {
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				backends = append(backends, Backend{
					Name:    node.Name,
					Address: addr.Address,
				})
				break
			}
		}
	}

	return backends, nil
}

type FrontendData struct {
	API      []string
	Ignition []string
	Ingress  []string
}

func addrFromVIP(addr *lb.VirtualAddress) (string, error) {
	a, err := netip.ParsePrefix(addr.Address)
	if err != nil {
		return "", fmt.Errorf("failed to parse VIP: %w", err)
	}
	if !a.IsSingleIP() {
		return "", fmt.Errorf("address ranges not supported for API VIP: %v", addr)
	}
	return a.Addr().String(), nil
}

func getFrontends(vips *lb.LoadBalancerConfigVIPs) (FrontendData, error) {
	var data FrontendData

	var errors []error

	for _, address := range vips.API {
		if addr, err := addrFromVIP(&address); err == nil {
			data.API = append(data.API, addr)
			if address.Type == lb.VirtualAddressPrivate {
				data.Ignition = append(data.Ignition, addr)
			}
		} else {
			errors = append(errors, err)
		}
	}

	for _, address := range vips.Ingress {
		if addr, err := addrFromVIP(&address); err == nil {
			data.Ingress = append(data.Ingress, addr)
		} else {
			errors = append(errors, err)
		}
	}

	return data, multierr.Combine(errors...)
}
