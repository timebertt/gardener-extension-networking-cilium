package charts

import (
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/extensions"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ciliumv1alpha1 "github.com/gardener/gardener-extension-networking-cilium/pkg/apis/cilium/v1alpha1"
)

var _ = Describe("#applyEncryptionConfig", func() {
	Describe("wireguard", func() {
		var config *ciliumv1alpha1.NetworkConfig
		BeforeEach(func() {
			config = &ciliumv1alpha1.NetworkConfig{
				Encryption: &ciliumv1alpha1.Encryption{
					Mode:    ciliumv1alpha1.EncryptionModeWireguard,
					Enabled: true,
				},
			}
		})
		Describe("vxlan tunnel config with strict mode encryption", func() {
			It("should set AllowRemoteNodeIdentities", func() {
				cfg := &globalConfig{
					Tunnel: ciliumv1alpha1.VXLan,
				}
				config.Encryption.StrictMode = true
				Expect(applyEncryptionConfig(cfg, config)).ShouldNot(HaveOccurred())
				Expect(cfg.Encryption.Wireguard.StrictMode.AllowRemoteNodeIdentities).To(BeTrue())
			})
		})
		Describe("overlapping node & pod CIDR with direct routing and strict mode encryption", func() {
			It("should set AllowRemoteNodeIdentities", func() {
				config.Encryption.StrictMode = true
				config.Overlay = &ciliumv1alpha1.Overlay{
					Enabled: false,
				}
				cfg := &globalConfig{
					Tunnel:   ciliumv1alpha1.Disabled,
					PodCIDR:  "10.0.0.0/16",
					NodeCIDR: "10.0.0.128/17",
				}
				Expect(applyEncryptionConfig(cfg, config)).ShouldNot(HaveOccurred())
				Expect(cfg.Encryption.Wireguard.StrictMode.AllowRemoteNodeIdentities).To(BeTrue())
			})
		})
	})
})

var _ = Describe("#generateChartValues", func() {
	Describe("hubble", func() {
		var config *ciliumv1alpha1.NetworkConfig
		cluster := &extensions.Cluster{
			Shoot: &gardencorev1beta1.Shoot{},
		}
		BeforeEach(func() {
			config = &ciliumv1alpha1.NetworkConfig{}
		})
		Describe("reflect hubble of NetworkConfig in both requirementsConfig and globalConfig", func() {
			It("should be disabled in requirementsConfig and globalConfig", func() {
				config.Hubble = &ciliumv1alpha1.Hubble{
					Enabled: false,
				}
				requirementCfg, globalCfg, err := generateChartValues(config, &extensionsv1alpha1.Network{}, cluster, "", "", "")
				Expect(err).NotTo(HaveOccurred())
				Expect(requirementCfg.Hubble.Enabled).To(Equal(config.Hubble.Enabled), "requirementsConfig mismatch")
				Expect(globalCfg.Hubble.Enabled).To(Equal(config.Hubble.Enabled), "globalConfig mismatch")
			})
		})
	})

	Describe("isMigratingCNI", func() {
		newCluster := func(specType, statusType string) *extensions.Cluster {
			return &extensions.Cluster{
				Shoot: &gardencorev1beta1.Shoot{
					Spec:   gardencorev1beta1.ShootSpec{Networking: &gardencorev1beta1.Networking{Type: &specType}},
					Status: gardencorev1beta1.ShootStatus{Networking: &gardencorev1beta1.NetworkingStatus{Type: &statusType}},
				},
			}
		}

		It("should be false when spec and status networking type are equal", func() {
			_, globalCfg, err := generateChartValues(&ciliumv1alpha1.NetworkConfig{}, &extensionsv1alpha1.Network{}, newCluster("cilium", "cilium"), "", "", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(globalCfg.IsMigratingCNI).To(BeFalse())
		})

		It("should be true when spec and status networking type differ", func() {
			_, globalCfg, err := generateChartValues(&ciliumv1alpha1.NetworkConfig{}, &extensionsv1alpha1.Network{}, newCluster("cilium", "calico"), "", "", "")
			Expect(err).NotTo(HaveOccurred())
			Expect(globalCfg.IsMigratingCNI).To(BeTrue())
		})
	})
})
