// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"context"
	"fmt"

	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	gardencorehelper "github.com/gardener/gardener/pkg/api/core/helper"
	"github.com/gardener/gardener/pkg/apis/core"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener-extension-networking-cilium/pkg/apis/cilium"
	"github.com/gardener/gardener-extension-networking-cilium/pkg/apis/cilium/v1alpha1"
	ciliumv1alpha1helper "github.com/gardener/gardener-extension-networking-cilium/pkg/apis/cilium/v1alpha1/helper"
	ciliumvalidation "github.com/gardener/gardener-extension-networking-cilium/pkg/apis/cilium/validation"
	ciliumtype "github.com/gardener/gardener-extension-networking-cilium/pkg/cilium"
)

// NewShootValidator returns a new instance of a shoot validator.
func NewShootValidator(mgr manager.Manager) extensionswebhook.Validator {
	return &shoot{
		client:         mgr.GetClient(),
		decoder:        serializer.NewCodecFactory(mgr.GetScheme(), serializer.EnableStrict).UniversalDecoder(),
		lenientDecoder: serializer.NewCodecFactory(mgr.GetScheme()).UniversalDecoder(),
	}
}

type shoot struct {
	client         client.Client
	decoder        runtime.Decoder
	lenientDecoder runtime.Decoder
}

// Validate validates the given shoot object.
func (s *shoot) Validate(ctx context.Context, newObj, old client.Object) error {
	shoot, ok := newObj.(*core.Shoot)
	if !ok {
		return fmt.Errorf("wrong object type %T", newObj)
	}

	// Skip if it's a workerless Shoot
	if gardencorehelper.IsWorkerless(shoot) {
		return nil
	}

	shootV1Beta1 := &gardencorev1beta1.Shoot{}
	err := gardencorev1beta1.Convert_core_Shoot_To_v1beta1_Shoot(shoot, shootV1Beta1, nil)
	if err != nil {
		return err
	}

	if old != nil {
		oldShoot, ok := old.(*core.Shoot)
		if !ok {
			return fmt.Errorf("wrong object type %T for old object", old)
		}
		return s.validateShootUpdate(ctx, oldShoot, shoot)
	}

	return s.validateShootCreation(ctx, shoot)
}

func (s *shoot) validateShoot(_ context.Context, shoot *core.Shoot) error {
	// Network validation
	if shoot.Spec.Networking != nil {
		if errList := ciliumvalidation.ValidateNetworking(shoot.Spec.Networking, field.NewPath("spec", "networking")); len(errList) != 0 {
			return errList.ToAggregate()
		}

		if shoot.Spec.Networking.ProviderConfig != nil {
			networkConfig, err := networkConfigFromShoot(shoot)
			if err != nil {
				return providerConfigDecodeError(shoot, err)
			}

			if networkConfig == nil {
				return nil
			}

			if errList := ciliumvalidation.ValidateNetworkConfig(networkConfig, field.NewPath("spec", "networking", "providerConfig")); len(errList) != 0 {
				return errList.ToAggregate()
			}
		}
	}

	return nil
}

func (s *shoot) validateShootUpdate(ctx context.Context, oldShoot, shoot *core.Shoot) error {
	if err := s.validateMigration(oldShoot, shoot); err != nil {
		return err
	}

	// Leaving Cilium means we don't need to validate the networking Config by this Extension.
	if networkingType(shoot) != ciliumtype.Type {
		return nil
	}

	return s.validateShoot(ctx, shoot)
}

func (s *shoot) validateShootCreation(ctx context.Context, shoot *core.Shoot) error {
	return s.validateShoot(ctx, shoot)
}

// validateMigration ensures that migrations to or from cilium only happen with native routing (overlay disabled),
// as the migration strategy relies on native routing and a routing controller.
func (s *shoot) validateMigration(oldShoot, shoot *core.Shoot) error {
	oldIsCilium := networkingType(oldShoot) == ciliumtype.Type
	newIsCilium := networkingType(shoot) == ciliumtype.Type
	if oldIsCilium == newIsCilium {
		return nil
	}

	// Leaving cilium: the old config is relevant. Entering cilium: the new one.
	target, msg := shoot, "overlay must be explicitly disabled (overlay.enabled=false) to switch to cilium"
	if oldIsCilium {
		target, msg = oldShoot, "cannot switch from cilium unless the overlay is explicitly disabled, set overlay.enabled=false first"
	}

	networkConfig, err := networkConfigFromShoot(target)
	if err != nil {
		return providerConfigDecodeError(target, err)
	}

	// Require explicit overlay off for strictness.
	if networkConfig == nil || networkConfig.Overlay == nil || networkConfig.Overlay.Enabled {
		return field.ErrorList{field.Forbidden(field.NewPath("spec", "networking", "providerConfig", "overlay"), msg)}.ToAggregate()
	}

	return nil
}

func networkingType(shoot *core.Shoot) string {
	if shoot.Spec.Networking == nil || shoot.Spec.Networking.Type == nil {
		return ""
	}
	return *shoot.Spec.Networking.Type
}

// networkConfigFromShoot decodes the shoot's networking providerConfig into the internal cilium NetworkConfig.
func networkConfigFromShoot(shoot *core.Shoot) (*cilium.NetworkConfig, error) {
	if shoot.Spec.Networking == nil || shoot.Spec.Networking.ProviderConfig == nil || len(shoot.Spec.Networking.ProviderConfig.Raw) == 0 {
		return nil, nil
	}

	network := &extensionsv1alpha1.Network{}
	network.Spec.ProviderConfig = shoot.Spec.Networking.ProviderConfig
	v1alpha1Config, err := ciliumv1alpha1helper.CiliumNetworkConfigFromNetworkResource(network)
	if err != nil {
		return nil, err
	}

	internalConfig := &cilium.NetworkConfig{}
	if err := v1alpha1.Convert_v1alpha1_NetworkConfig_To_cilium_NetworkConfig(v1alpha1Config, internalConfig, nil); err != nil {
		return nil, err
	}
	return internalConfig, nil
}

// providerConfigDecodeError wraps an error from decoding the shoot's networking providerConfig into a field error.
func providerConfigDecodeError(shoot *core.Shoot, err error) error {
	return field.ErrorList{
		field.Invalid(field.NewPath("spec", "networking", "providerConfig"), string(shoot.Spec.Networking.ProviderConfig.Raw), err.Error()),
	}.ToAggregate()
}
