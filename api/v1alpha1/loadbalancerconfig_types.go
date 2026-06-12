/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LoadBalancerConfigSpec defines the desired state of LoadBalancerConfig
type LoadBalancerConfigSpec struct {
	// Cloud provider for which the loadbalancers should be configured.
	//
	// +required
	// +kubebuilder:validation:Enum=cloudscale;exoscale
	Cloud string `json:"cloud"`

	// CloudCredentials references a secret which contains credentials for
	// the cloud provider selected via field `cloud`.
	//
	// +required
	CloudCredentials corev1.LocalObjectReference `json:"cloudCredentials"`

	// Distribution defines the set of HAProxy backends.
	//
	// +required
	// +kubebuilder:validation:Enum=openshift;talos
	Distribution string `json:"distribution"`

	// VirtualAddresses defines the IP addresses on which the LB operates.
	//
	// +required
	VirtualAddresses LoadBalancerConfigVIPs `json:"virtualAddresses"`

	// Kubernetes API server backend config.
	//
	// +required
	APIBackend LoadBalancerConfigBackend `json:"apiBackend"`

	// Kubernetes ingress controller backend config.
	//
	// +optional
	IngressBackend *LoadBalancerConfigBackend `json:"ingressBackend,omitzero"`

	// Custom Firewall rules.
	// The controller configures a default set of firewall rules based on
	// `VirtualAddresses`.
	//
	// +optional
	FirewallRules []string `json:"firewallRules,omitempty"`
}

type LoadBalancerConfigVIPs struct {
	// Virtual addresses for the Kubernetes API server
	//
	// +required
	// +listType=map
	// +listMapKey=address
	// +kubebuilder:validation:MinItems=1
	API []VirtualAddress `json:"api"`

	// Virtual addresses for the ingress controller
	//
	// If omitted, the HAproxy ingress section won't be configured.
	//
	// +optional
	// +listType=map
	// +listMapKey=address
	Ingress []VirtualAddress `json:"ingress,omitempty"`

	// Virtual address for the default gateway SNAT source.
	//
	// If omitted, the LBs are configured without NAT gateway
	// functionality.
	//
	// +optional
	NAT *VirtualAddress `json:"nat,omitzero"`
}

const (
	AddressTypePrivate = "private"
	AddressTypePublic  = "public"
)

type VirtualAddress struct {
	// Type indicates whether the address is a public or private VIP
	//
	// Public addresses are managed by Floaty, private addresses are
	// managed by keepalived.
	//
	// +kubebuilder:validation:Enum=public;private
	Type string `json:"type"`

	// Address is the address in CIDR notation.
	Address string `json:"address"`
}

type LoadBalancerConfigBackend struct {
	// NodeSelector selects the cluster nodes which the LB HAProxy API
	// backends should target.
	NodeSelector metav1.LabelSelector `json:"nodeSelector"`
}

// LoadBalancerConfigStatus defines the observed state of LoadBalancerConfig.
type LoadBalancerConfigStatus struct {
	// nodes tracks the status of each LB node that's configured through
	// this LoadBalancerConfig resource.
	Nodes map[string]*LoadBalancerNodeStatus `json:"nodes,omitempty"`

	// conditions represent the current state of the LoadBalancerConfig resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type LoadBalancerNodeStatus struct {
	// The node's public IP
	// +required
	PublicIP string `json:"publicIP"`

	// The node's private IP
	// +required
	PrivateIP string `json:"privateIP"`

	// The node's VRRP peer IP
	// +required
	VrrpIP string `json:"vrrpIP"`

	// A map of config file names to sha256 hashes of the last rendered config.
	//
	// The contents of this map are used by the controller to decide
	// whether to write new versions of each config file and whether the
	// configured service should be restarted/reloaded.
	ConfigHashes map[string]string `json:"configHashes,omitempty"`

	// The LB node's status.
	// +kubebuilder:validation:Enum=Configuring;Ready;Failed
	Status string `json:"status"`
}

const (
	NodeStatusConfiguring = "Configuring"
	NodeStatusReady       = "Ready"
	NodeStatusFailed      = "Failed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// LoadBalancerConfig is the Schema for the loadbalancerconfigs API
type LoadBalancerConfig struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of LoadBalancerConfig
	// +required
	Spec LoadBalancerConfigSpec `json:"spec"`

	// status defines the observed state of LoadBalancerConfig
	// +optional
	Status LoadBalancerConfigStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// LoadBalancerConfigList contains a list of LoadBalancerConfig
type LoadBalancerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []LoadBalancerConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LoadBalancerConfig{}, &LoadBalancerConfigList{})
}
