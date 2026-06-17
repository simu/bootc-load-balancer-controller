package controller

import (
	"context"
	"net/netip"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"tailscale.com/net/netmon"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1alpha1 "github.com/simu/bootc-load-balancer-controller/api/v1alpha1"
)

var _ = Describe("LoadBalancerConfig Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		// TODO(sg): what's the right way to
		configRoot, err := os.MkdirTemp("/tmp", "bootc-lb-test-*")
		Expect(err).NotTo(HaveOccurred())
		detectedPublicInterface, err := netmon.DefaultRouteInterface()
		Expect(err).NotTo(HaveOccurred())
		// NOTE(sg): use primary iface net as cluster net to avoid
		// issues in CI
		primaryIP, err := primaryIPAddressForInterface(detectedPublicInterface, nil)
		Expect(err).NotTo(HaveOccurred())
		pip, err := netip.ParsePrefix(primaryIP)
		Expect(err).NotTo(HaveOccurred())
		clusterNet := pip.Masked()

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		loadbalancerconfig := &configv1alpha1.LoadBalancerConfig{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind LoadBalancerConfig")
			err := k8sClient.Get(ctx, typeNamespacedName, loadbalancerconfig)
			if err != nil && errors.IsNotFound(err) {
				creds := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cloudscale-token",
						Namespace: "default",
					},
					Data: map[string][]byte{
						"token": []byte("verysecrettoken"),
					},
				}
				Expect(k8sClient.Create(ctx, creds)).To(Succeed())
				resource := &configv1alpha1.LoadBalancerConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: configv1alpha1.LoadBalancerConfigSpec{
						Cloud: "cloudscale",
						CloudCredentials: corev1.LocalObjectReference{
							Name: creds.Name,
						},
						Distribution: "openshift",
						VirtualAddresses: configv1alpha1.LoadBalancerConfigVIPs{
							API: []configv1alpha1.VirtualAddress{
								{
									Type:    "public",
									Address: "198.51.100.100/32",
								},
							},
							Ingress: []configv1alpha1.VirtualAddress{
								{
									Type:    "public",
									Address: "198.51.100.80/32",
								},
							},
							NAT: &configv1alpha1.VirtualAddress{
								Type:    "public",
								Address: "198.51.100.253/32",
							},
						},
						APIBackend: configv1alpha1.LoadBalancerConfigBackend{
							NodeSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"node-role.kubernetes.io/control-plane": "",
								},
							},
						},
						IngressBackend: &configv1alpha1.LoadBalancerConfigBackend{
							NodeSelector: metav1.LabelSelector{
								MatchLabels: map[string]string{
									"node-role.kubernetes.io/infra": "",
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &configv1alpha1.LoadBalancerConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance LoadBalancerConfig")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &LoadBalancerConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),

				ConfigRoot:      configRoot,
				PublicInterface: detectedPublicInterface,
				ClusterNetwork:  clusterNet,
				WatchNamespace:  "default",
				Hostname:        "lb1",
				KeepalivedConfig: KeepalivedConfig{
					IsPrimary: false,
				},
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
