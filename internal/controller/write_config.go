package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

	digestExt = ".sha256"
)

func RenderFinalConfig(cfgfilepath string, lbconfigMeta *metav1.ObjectMeta, configdata string) (string, error) {
	header := fileHeader
	stamp := stampYAML
	if filepath.Ext(cfgfilepath) == ".xml" {
		header = xmlFileHeader
		stamp = stampXML
	}

	var cfgfilecontents strings.Builder

	if _, err := cfgfilecontents.WriteString(header); err != nil {
		return "", fmt.Errorf("failed to render file header: %w", err)
	}
	if _, err := stamp(&cfgfilecontents, lbconfigMeta.Namespace, lbconfigMeta.Name, lbconfigMeta.UID, lbconfigMeta.Generation); err != nil {
		return "", fmt.Errorf("failed to render LoadBalancerConfig referer: %w", err)
	}

	if _, err := cfgfilecontents.WriteString(configdata); err != nil {
		return "", fmt.Errorf("failed to render config data: %w", err)
	}

	return cfgfilecontents.String(), nil
}

func (r *LoadBalancerConfigReconciler) WriteConfig(ctx context.Context, lbconfigMeta *metav1.ObjectMeta, configfile, configdata, digestdata string, mode os.FileMode) error {
	l := logf.FromContext(ctx)

	if mode == 0 {
		mode = 0644
	}
	cfgfilepath := r.configfilePath(configfile)
	digestfilepath := r.digestfilePath(configfile)

	l.Info("Rendering final config file", "file", configfile)
	cfgfilecontents, err := RenderFinalConfig(cfgfilepath, lbconfigMeta, configdata)
	if err != nil {
		return fmt.Errorf("failed to render final file contents: %w", err)
	}

	l.Info("Writing config file", "file", configfile, "path", cfgfilepath, "mode", mode)

	if err := os.MkdirAll(filepath.Dir(cfgfilepath), 0755); err != nil {
		return fmt.Errorf("failed to create director for config file: %w", err)
	}
	cfgfile, err := os.OpenFile(cfgfilepath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create or open config file: %w", err)
	}

	if _, err := cfgfile.WriteString(cfgfilecontents); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	if err := cfgfile.Close(); err != nil {
		return fmt.Errorf("failed to close config file: %w", err)
	}

	digestfile, err := os.OpenFile(digestfilepath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create or open config digest file: %w", err)
	}

	if _, err := fmt.Fprintf(digestfile, "%x", sha256.Sum256([]byte(cfgfilecontents))); err != nil {
		return fmt.Errorf("failed to write config file digest: %w", err)
	}

	if err := digestfile.Close(); err != nil {
		return fmt.Errorf("failed to close config digest file: %w", err)
	}

	return nil
}

func (r *LoadBalancerConfigReconciler) IsFileModified(ctx context.Context, configfile string) bool {
	l := logf.FromContext(ctx)

	cfgfile := r.configfilePath(configfile)
	config, err := os.ReadFile(cfgfile)
	if err != nil {
		// if we can't read the configfile for any reason we always overwrite it
		return true
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(config))

	digestfile := r.digestfilePath(configfile)
	storeddigest, err := os.ReadFile(digestfile)
	if err != nil {
		// if we can't read the digestfile for any reason we always
		// overwrite the configfile
		return true
	}

	l.Info("on-disk configfile", "configfile", configfile, "digest", digest, "stored digest", string(storeddigest))

	return digest != string(storeddigest)
}

func (r *LoadBalancerConfigReconciler) configfilePath(configfile string) string {
	return filepath.Join(r.ConfigRoot, configfile)
}

func (r *LoadBalancerConfigReconciler) digestfilePath(configfile string) string {
	cfgfilepath := r.configfilePath(configfile)
	return fmt.Sprintf("%s%s", cfgfilepath, digestExt)
}

func stampYAML(w io.Writer, a ...any) (int, error) {
	return fmt.Fprintf(w, "# Generated from LoadBalancerConfig %s/%s (UID %s, generation %d)\n\n", a...)
}

func stampXML(w io.Writer, a ...any) (int, error) {
	return fmt.Fprintf(w, "<!-- Generated from LoadBalancerConfig %s/%s (UID %s, generation %d)-->\n\n", a...)
}
