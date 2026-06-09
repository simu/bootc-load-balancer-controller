package controller

import (
	"context"
	"fmt"
	"net/netip"
	"slices"

	"golang.org/x/exp/maps"

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
	API      []netip.Addr
	Ignition []netip.Addr
	Ingress  []netip.Addr
}

type InternalIPs struct {
	ClusterNetwork netip.Prefix
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
	api := map[netip.Addr]struct{}{}
	ignition := map[netip.Addr]struct{}{}
	ingress := map[netip.Addr]struct{}{}

	var errors []error

	for _, address := range vips.API {
		if addr, err := parseVIP(&address); err == nil {
			api[addr.Addr()] = struct{}{}
			if address.Type == lb.AddressTypePrivate {
				ignition[addr.Addr()] = struct{}{}
			}
		} else {
			errors = append(errors, err)
		}
	}

	for _, address := range vips.Ingress {
		if addr, err := parseVIP(&address); err == nil {
			ingress[addr.Addr()] = struct{}{}
		} else {
			errors = append(errors, err)
		}
	}

	data := FrontendData{
		API:      maps.Keys(api),
		Ignition: maps.Keys(ignition),
		Ingress:  maps.Keys(ingress),
	}
	sortIPs(data.API)
	sortIPs(data.Ignition)
	sortIPs(data.Ingress)
	return data, multierr.Combine(errors...)
}

func privateVIPs(vips *lb.LoadBalancerConfigVIPs, additionalVIPs ...netip.Prefix) ([]netip.Prefix, error) {
	addrs := map[netip.Prefix]struct{}{}
	for _, a := range additionalVIPs {
		addrs[a] = struct{}{}
	}
	errors := []error{}

	for _, address := range vips.API {
		if address.Type != lb.AddressTypePrivate {
			continue
		}
		if a, err := parseVIP(&address); err == nil {
			addrs[a] = struct{}{}
		} else {
			errors = append(errors, err)
		}
	}
	for _, address := range vips.Ingress {
		if address.Type != lb.AddressTypePrivate {
			continue
		}
		if a, err := parseVIP(&address); err == nil {
			addrs[a] = struct{}{}
		} else {
			errors = append(errors, err)
		}
	}
	if vips.NAT != nil && vips.NAT.Type == lb.AddressTypePrivate {
		if a, err := parseVIP(vips.NAT); err == nil {
			addrs[a] = struct{}{}
		} else {
			errors = append(errors, err)
		}
	}

	retAddrs := maps.Keys(addrs)
	sortPrefixes(retAddrs)
	return retAddrs, multierr.Combine(errors...)
}

func publicVIPs(vips *lb.LoadBalancerConfigVIPs) ([]netip.Prefix, error) {
	addrs := map[netip.Prefix]struct{}{}
	errors := []error{}

	for _, address := range vips.API {
		if address.Type != lb.AddressTypePublic {
			continue
		}
		if a, err := parseVIP(&address); err == nil {
			addrs[a] = struct{}{}
		} else {
			errors = append(errors, err)
		}
	}
	for _, address := range vips.Ingress {
		if address.Type != lb.AddressTypePublic {
			continue
		}
		if a, err := parseVIP(&address); err == nil {
			addrs[a] = struct{}{}
		} else {
			errors = append(errors, err)
		}
	}
	if vips.NAT != nil && vips.NAT.Type == lb.AddressTypePublic {
		if a, err := parseVIP(vips.NAT); err == nil {
			addrs[a] = struct{}{}
		} else {
			errors = append(errors, err)
		}
	}

	retAddrs := maps.Keys(addrs)
	sortPrefixes(retAddrs)
	return retAddrs, multierr.Combine(errors...)
}

func parseVIP(addr *lb.VirtualAddress) (netip.Prefix, error) {
	a, err := netip.ParsePrefix(addr.Address)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("failed to parse VIP: %w", err)
	}
	if !a.IsSingleIP() {
		return netip.Prefix{}, fmt.Errorf("address ranges not supported as VIP: %v", addr)
	}
	return a, nil
}

func (r *LoadBalancerConfigReconciler) internalIPs() (*InternalIPs, error) {
	netaddr := r.ClusterNetwork.Masked().Addr()
	if !netaddr.Is4() {
		return nil, fmt.Errorf("IPv6 cluster network isn't supported: %s", r.ClusterNetwork)
	}
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

func sortIPs(ips []netip.Addr) {
	slices.SortFunc(ips, func(a, b netip.Addr) int {
		return a.Compare(b)
	})
}

func sortPrefixes(pfx []netip.Prefix) {
	slices.SortFunc(pfx, func(a, b netip.Prefix) int {
		return a.Addr().Compare(b.Addr())
	})
}
