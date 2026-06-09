package controller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	ConntrackdConfigFile     = "/etc/conntrackd/conntrackd.conf"
	FloatyConfigFile         = "/etc/floaty/global.yaml"
	HAProxyAPIConfigFile     = "/etc/haproxy/conf.d/api.cfg"
	HAProxyIngressConfigFile = "/etc/haproxy/conf.d/ingress.cfg"
	KeepalivedConfigFile     = "/etc/keepalived/keepalived.conf"

	fileHeader = "# Managed by bootc-loadbalancer-controller\n"
)

func (r *LoadBalancerConfigReconciler) WriteConfig(ctx context.Context, lbconfigMeta *metav1.ObjectMeta, configfile, configdata string) error {
	l := logf.FromContext(ctx)

	cfgfilepath := filepath.Join(r.ConfigRoot, configfile)
	l.Info("Writing config file", "file", configfile, "path", cfgfilepath)

	if err := os.MkdirAll(filepath.Dir(cfgfilepath), 0755); err != nil {
		return fmt.Errorf("failed to create director for config file: %w", err)
	}

	cfgfile, err := os.OpenFile(cfgfilepath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to create or open config file: %w", err)
	}

	if _, err := cfgfile.WriteString(fileHeader); err != nil {
		return fmt.Errorf("failed to write file header: %w", err)
	}
	if _, err := fmt.Fprintf(cfgfile, "# Generated from LoadBalancerConfig %s/%s (UID %s, generation %d)\n\n", lbconfigMeta.Namespace, lbconfigMeta.Name, lbconfigMeta.UID, lbconfigMeta.Generation); err != nil {
		return fmt.Errorf("failed to write LoadBalancerConfig referer: %w", err)
	}

	if _, err := cfgfile.WriteString(configdata); err != nil {
		return fmt.Errorf("failed to write config data: %w", err)
	}

	if err := cfgfile.Close(); err != nil {
		return fmt.Errorf("failed to close config file: %w", err)
	}

	return nil
}
