// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"context"

	"github.com/gardener/gardener/pkg/apis/core"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("Shoot validator migration", func() {
	const (
		nativeRouting = `{"apiVersion":"cilium.networking.extensions.gardener.cloud/v1alpha1","kind":"NetworkConfig","overlay":{"enabled":false}}`
		overlay       = `{"apiVersion":"cilium.networking.extensions.gardener.cloud/v1alpha1","kind":"NetworkConfig","overlay":{"enabled":true}}`
		noOverlayKey  = `{"apiVersion":"cilium.networking.extensions.gardener.cloud/v1alpha1","kind":"NetworkConfig"}`
		unknownField  = `{"apiVersion":"cilium.networking.extensions.gardener.cloud/v1alpha1","kind":"NetworkConfig","overlay":{"enabled":false},"doesNotExist":true}`
	)

	var s *shoot

	newShoot := func(networkingType, providerConfig string) *core.Shoot {
		networking := &core.Networking{
			Type:  new(networkingType),
			Pods:  new("10.0.0.0/16"),
			Nodes: new("10.1.0.0/16"),
		}
		if providerConfig != "" {
			networking.ProviderConfig = &runtime.RawExtension{Raw: []byte(providerConfig)}
		}
		return &core.Shoot{Spec: core.ShootSpec{Networking: networking}}
	}

	BeforeEach(func() {
		s = &shoot{}
	})

	DescribeTable("#validateMigration",
		func(oldType, oldConfig, newType, newConfig string, matcher OmegaMatcher) {
			Expect(s.validateMigration(newShoot(oldType, oldConfig), newShoot(newType, newConfig))).To(matcher)
		},

		// no migration
		Entry("cilium to cilium without overlay config", "cilium", "", "cilium", "", Succeed()),
		Entry("cilium to cilium with overlay", "cilium", overlay, "cilium", overlay, Succeed()),
		Entry("other to other", "calico", overlay, "other", "", Succeed()),

		// entering cilium: the new config is checked
		Entry("to cilium with native routing", "calico", "", "cilium", nativeRouting, Succeed()),
		Entry("to cilium with overlay", "calico", "", "cilium", overlay, MatchError(ContainSubstring("overlay"))),
		Entry("to cilium without overlay setting", "calico", "", "cilium", noOverlayKey, MatchError(ContainSubstring("overlay"))),
		Entry("to cilium without providerConfig", "calico", "", "cilium", "", MatchError(ContainSubstring("overlay"))),
		// the other Extension (here calico) is responsible for checking the own config
		Entry("to cilium ignores old config", "calico", overlay, "cilium", nativeRouting, Succeed()),
		Entry("to cilium rejects unknown fields in the new config", "calico", "", "cilium", unknownField, HaveOccurred()),

		// leaving cilium: the old config is checked
		Entry("from cilium with native routing", "cilium", nativeRouting, "calico", "", Succeed()),
		Entry("from cilium with overlay", "cilium", overlay, "calico", "", MatchError(ContainSubstring("overlay"))),
		Entry("from cilium without providerConfig", "cilium", "", "calico", "", MatchError(ContainSubstring("overlay"))),
		Entry("from cilium ignores new config", "cilium", nativeRouting, "calico", overlay, Succeed()),
	)

	Describe("#validateShootUpdate", func() {
		It("should not run cilium validation on a shoot that leaves cilium", func() {
			// The new providerConfig is a calico config, which cilium's strict decoding would reject.
			calico := `{"apiVersion":"calico.networking.extensions.gardener.cloud/v1alpha1","kind":"NetworkConfig"}`
			Expect(s.validateShootUpdate(context.Background(), newShoot("cilium", nativeRouting), newShoot("calico", calico))).To(Succeed())
		})

		It("should still validate cilium shoots that are not migrating", func() {
			invalid := newShoot("cilium", nativeRouting)
			invalid.Spec.Networking.Pods = new("not-a-cidr")
			Expect(s.validateShootUpdate(context.Background(), newShoot("cilium", nativeRouting), invalid)).To(MatchError(ContainSubstring("pods")))
		})
	})

})
