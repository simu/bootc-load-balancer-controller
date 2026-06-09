package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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
		return "", fmt.Errorf("failed to compute internal VIPs: %w", err)
	}

	prio := 100
	if r.KeepalivedConfig.IsPrimary {
		prio = 200
	}

	vips, err := privateVIPs(&lbconfig.Spec.VirtualAddresses)
	if err != nil {
		return "", fmt.Errorf("failed to prepare Keepalived VIPs: %w", err)
	}

	if lbconfig.Spec.VirtualAddresses.NAT != nil {
		vips = append(vips, internalIPs.DefaultGateway)
	}

	l.Info("Rendering keepalived config", "interface", r.KeepalivedConfig.Interface, "priority", prio)
	return renderTemplate(lbconfig.Spec.Distribution, "keepalived.conf.tmpl", map[string]any{
		"Interface": r.KeepalivedConfig.Interface,
		"Priority":  prio,
		"SrcIP":     internalIPs.myInternalIP(r.KeepalivedConfig.IsPrimary, false),
		"DstIP":     internalIPs.peerInternalIP(r.KeepalivedConfig.IsPrimary, false),
		"VIPs":      vips,
	})
}

func (r *LoadBalancerConfigReconciler) RenderFloatyConfig(ctx context.Context, lbconfig *lb.LoadBalancerConfig) (string, error) {
	l := logf.FromContext(ctx)

	var credentialSecret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: lbconfig.Namespace,
		Name:      lbconfig.Spec.CloudCredentials.Name,
	}, &credentialSecret); err != nil {
		return "", fmt.Errorf("failed to read cloud provider credentials: %w", err)
	}

	vips, err := publicVIPs(&lbconfig.Spec.VirtualAddresses)
	if err != nil {
		return "", fmt.Errorf("failed to compute public floating IPs: %w", err)
	}

	l.Info("Rendering Floaty config", "provider", lbconfig.Spec.Cloud)

	// TODO(sg): might make sense to make Floaty config types public
	// rather than go-templating YAML here.
	templateData := map[string]any{
		"VIPs": vips,
	}
	switch lbconfig.Spec.Cloud {
	case "cloudscale":
		if token, ok := credentialSecret.Data["token"]; ok {
			templateData["Token"] = string(token)
		} else {
			return "", fmt.Errorf("cloudscale credentials secret is missing field 'token'")
		}
	case "exoscale":
		if key, ok := credentialSecret.Data["key"]; ok {
			templateData["Key"] = string(key)
		} else {
			return "", fmt.Errorf("exoscale credentials secret is missing field 'key'")
		}
		if secret, ok := credentialSecret.Data["secret"]; ok {
			templateData["Secret"] = string(secret)
		} else {
			return "", fmt.Errorf("exoscale credentials secret is missing field 'secret'")
		}
	default:
		return "", fmt.Errorf("unknown cloud provider '%s'", lbconfig.Spec.Cloud)
	}
	return renderTemplate(lbconfig.Spec.Cloud, "floaty.yaml.tmpl", templateData)
}
