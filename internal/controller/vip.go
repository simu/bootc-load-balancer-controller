package controller

import (
	"context"
	"fmt"
	"net/netip"
	"reflect"

	lb "github.com/projectsyn/bootc-load-balancer-controller/api/v1alpha1"
	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Backend struct {
	Name    string
	Address string
}

type FrontendData struct {
	API      []string
	Ignition []string
	Ingress  []string
}

type InternalIPs struct {
	DefaultGateway netip.Addr
	Primary        netip.Addr
	Secondary      netip.Addr
}

func (r *LoadBalancerConfigReconciler) getHAProxyBackends(ctx context.Context, ls *metav1.LabelSelector) ([]Backend, error) {
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

func getHAProxyFrontends(vips *lb.LoadBalancerConfigVIPs) (FrontendData, error) {
	var data FrontendData

	var errors []error

	for _, address := range vips.API {
		if addr, err := addrFromVIP[string](&address); err == nil {
			data.API = append(data.API, addr)
			if address.Type == lb.AddressTypePrivate {
				data.Ignition = append(data.Ignition, addr)
			}
		} else {
			errors = append(errors, err)
		}
	}

	for _, address := range vips.Ingress {
		if addr, err := addrFromVIP[string](&address); err == nil {
			data.Ingress = append(data.Ingress, addr)
		} else {
			errors = append(errors, err)
		}
	}

	return data, multierr.Combine(errors...)
}

func privateVIPs(vips *lb.LoadBalancerConfigVIPs, cidr bool) ([]string, error) {
	addrs := []string{}
	errors := []error{}

	for _, address := range vips.API {
		if address.Type != lb.AddressTypePrivate {
			continue
		}
		if a, err := addrFromVIP[string](&address); err == nil {
			addrs = append(addrs, a)
		} else {
			errors = append(errors, err)
		}
	}
	for _, address := range vips.Ingress {
		if address.Type != lb.AddressTypePrivate {
			continue
		}
		if a, err := addrFromVIP[string](&address); err == nil {
			addrs = append(addrs, a)
		} else {
			errors = append(errors, err)
		}
	}
	if vips.NAT != nil && vips.NAT.Type == lb.AddressTypePrivate {
		if a, err := addrFromVIP[string](vips.NAT); err == nil {
			addrs = append(addrs, a)
		} else {
			errors = append(errors, err)
		}
	}

	return addrs, multierr.Combine(errors...)
}

func publicVIPs[T netip.Addr | netip.Prefix](vips *lb.LoadBalancerConfigVIPs, cidr bool) ([]T, error) {
	addrs := []T{}
	errors := []error{}

	for _, address := range vips.API {
		if address.Type != lb.AddressTypePublic {
			continue
		}
		if a, err := addrFromVIP[T](&address); err == nil {
			addrs = append(addrs, a)
		} else {
			errors = append(errors, err)
		}
	}
	for _, address := range vips.Ingress {
		if address.Type != lb.AddressTypePublic {
			continue
		}
		if a, err := addrFromVIP[T](&address); err == nil {
			addrs = append(addrs, a)
		} else {
			errors = append(errors, err)
		}
	}
	if vips.NAT != nil && vips.NAT.Type == lb.AddressTypePublic {
		if a, err := addrFromVIP[T](vips.NAT); err == nil {
			addrs = append(addrs, a)
		} else {
			errors = append(errors, err)
		}
	}

	return addrs, multierr.Combine(errors...)
}

type AddrOutput interface {
	netip.Addr | netip.Prefix | string
}

// TODO(sg): Refactor the whole netip handling. I don't think you're supposed
// to do this in Go ^^. Also, we don't need to preconvert to strings, that's
// done automatically by `text/template`.
func addrFromVIP[T AddrOutput](addr *lb.VirtualAddress) (res T, err error) {
	a, e := netip.ParsePrefix(addr.Address)
	if e != nil {
		err = fmt.Errorf("failed to parse VIP: %w", e)
		return
	}
	if !a.IsSingleIP() {
		err = fmt.Errorf("address ranges not supported as VIP: %v", addr)
		return
	}
	err = nil
	if r, ok := any(a).(T); ok {
		return r, nil
	} else if r, ok := any(a.Addr()).(T); ok {
		return r, nil
	} else if r, ok := any(a.Addr().String()).(T); ok {
		return r, nil
	} else {
		err = fmt.Errorf("unable to generate requested return type: %v", reflect.TypeOf(res).String())
		return
	}
}

func (r *LoadBalancerConfigReconciler) internalIPs(lbconfig *lb.LoadBalancerConfig) (*InternalIPs, error) {
	clusternet, err := netip.ParsePrefix(lbconfig.Spec.ClusterNetwork)
	if err != nil {
		return nil, fmt.Errorf("failed to parse cluster network: %w", err)
	}
	netaddr := clusternet.Masked().Addr()
	defaultGateway := netaddr.Next()
	primaryIP := defaultGateway.Next()
	secondaryIP := primaryIP.Next()
	return &InternalIPs{
		DefaultGateway: defaultGateway,
		Primary:        primaryIP,
		Secondary:      secondaryIP,
	}, nil
}

func (ip *InternalIPs) myInternalIP(isPrimary bool) string {
	if isPrimary {
		return ip.Primary.String()
	}
	return ip.Secondary.String()
}

func (ip *InternalIPs) peerInternalIP(isPrimary bool) string {
	if isPrimary {
		return ip.Secondary.String()
	}
	return ip.Primary.String()
}
