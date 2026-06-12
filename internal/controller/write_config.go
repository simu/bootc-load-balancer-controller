package controller

import (
	"context"
	"fmt"
	"io"
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
	KeepalivedConfigFile     = "/etc/keepalived/conf.d/lb.conf"

	PublicNMConnectionFile          = "/etc/NetworkManager/system-connections/public.nmconnection"
	ClusterNetworkNMConnectionFile  = "/etc/NetworkManager/system-connections/cluster-net.nmconnection"
	KeepalivedDummyNMConnectionFile = "/etc/NetworkManager/system-connections/keepalived.nmconnection"

	SysctlConfFile = "/etc/sysctl.d/50-lb.conf"

	FirewalldDirectFile   = "/etc/firewalld/direct.xml"
	FirewalldExternalZone = "/etc/firewalld/zones/external.xml"

	fileHeader    = "# Managed by bootc-loadbalancer-controller\n"
	xmlFileHeader = "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n\n<!-- Managed by bootc-loadbalancer-controller -->\n"
)

func (r *LoadBalancerConfigReconciler) WriteConfig(ctx context.Context, lbconfigMeta *metav1.ObjectMeta, configfile, configdata string, mode os.FileMode) error {
	l := logf.FromContext(ctx)

	if mode == 0 {
		mode = 0644
	}
	cfgfilepath := filepath.Join(r.ConfigRoot, configfile)

	l.Info("Writing config file", "file", configfile, "path", cfgfilepath, "mode", mode)

	header := fileHeader
	stamp := stampYAML
	if filepath.Ext(cfgfilepath) == ".xml" {
		header = xmlFileHeader
		stamp = stampXML
	}

	if err := os.MkdirAll(filepath.Dir(cfgfilepath), 0755); err != nil {
		return fmt.Errorf("failed to create director for config file: %w", err)
	}
	cfgfile, err := os.OpenFile(cfgfilepath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create or open config file: %w", err)
	}

	if _, err := cfgfile.WriteString(header); err != nil {
		return fmt.Errorf("failed to write file header: %w", err)
	}
	if _, err := stamp(cfgfile, lbconfigMeta.Namespace, lbconfigMeta.Name, lbconfigMeta.UID, lbconfigMeta.Generation); err != nil {
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

func stampYAML(w io.Writer, a ...any) (int, error) {
	return fmt.Fprintf(w, "# Generated from LoadBalancerConfig %s/%s (UID %s, generation %d)\n\n", a...)
}

func stampXML(w io.Writer, a ...any) (int, error) {
	return fmt.Fprintf(w, "<!-- Generated from LoadBalancerConfig %s/%s (UID %s, generation %d)-->\n\n", a...)
}
