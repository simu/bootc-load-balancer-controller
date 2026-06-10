package render

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	lb "github.com/projectsyn/bootc-load-balancer-controller/api/v1alpha1"
	"github.com/projectsyn/bootc-load-balancer-controller/internal/controller"
)

func parseBackendString(prefix, backends string) ([]controller.Backend, error) {
	belist := strings.Split(backends, ",")
	parsed := make([]controller.Backend, len(belist))
	for i, be := range belist {
		beip, err := netip.ParseAddr(be)
		if err != nil {
			return []controller.Backend{}, fmt.Errorf("failed to parse backend ip %s for %s: %w", be, prefix, err)
		}
		parsed[i] = controller.Backend{
			Name:    fmt.Sprintf("%s_%d", prefix, i),
			Address: beip.String(),
		}
	}
	return parsed, nil
}

func RenderFile(r *controller.LoadBalancerConfigReconciler, file, apiBackends, ingressBackends string) error {
	raw, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("failed to read managed resource file: %w", err)
	}

	api, err := parseBackendString("api", apiBackends)
	if err != nil {
		return err
	}
	ingress, err := parseBackendString("ingress", apiBackends)
	if err != nil {
		return err
	}

	yr := yaml.NewYAMLReader(bufio.NewReader(bytes.NewBuffer(raw)))

	var lbconfig lb.LoadBalancerConfig
	var creds corev1.Secret

	for {
		if doc, err := yr.Read(); err == nil {
			if err := yaml.UnmarshalStrict(doc, &lbconfig); err == nil {
				fmt.Println("Found LoadBalancerConfig")
			}
			if err := yaml.UnmarshalStrict(doc, &creds); err == nil {
				fmt.Println("Found Secret")
			}
		} else if err == io.EOF {
			break
		} else {
			fmt.Println(doc, err)
			return fmt.Errorf("failed to read input file: %w", err)
		}
	}

	if creds.Data == nil {
		creds.Data = map[string][]byte{}
	}
	for k, v := range creds.StringData {
		creds.Data[k] = []byte(v)
	}

	fmt.Printf("%v\n", creds.Data)

	_, err = r.ReconcileLBConfig(context.Background(), &lbconfig, &creds, &controller.LocalBackendConfiguration{
		API:     api,
		Ingress: ingress,
	})
	if err != nil {
		return fmt.Errorf("failed to render loadbalancerconfig file: %w", err)
	}

	return nil
}
