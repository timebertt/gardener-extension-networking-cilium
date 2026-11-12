// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package charts

import (
	"fmt"

	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/gardener-extension-networking-cilium/charts"
	ciliumv1alpha1 "github.com/gardener/gardener-extension-networking-cilium/pkg/apis/cilium/v1alpha1"
	"github.com/gardener/gardener-extension-networking-cilium/pkg/cilium"
)

// CiliumConfigKey defines the cilium configmap key.
const CiliumConfigKey = "config.yaml"

// RenderCiliumChart renders the cilium chart with the given values.
func RenderCiliumChart(renderer chartrenderer.Interface, config *ciliumv1alpha1.NetworkConfig, network *extensionsv1alpha1.Network, cluster *extensionscontroller.Cluster, ipamMode, configMapHash, configMapLabelPrefixHash string) ([]byte, error) {
	var release *chartrenderer.RenderedChart

	values, err := ComputeCiliumChartValues(config, network, cluster, ipamMode, configMapHash, configMapLabelPrefixHash)
	if err != nil {
		return nil, err
	}

	release, err = renderer.RenderEmbeddedFS(charts.InternalChart, cilium.CiliumChartPath, cilium.ReleaseName, metav1.NamespaceSystem, values)
	if err != nil {
		return nil, err
	}

	configMapPath := "cilium/charts/config/templates/configmap.yaml"
	newConfigMapHash, err := getConfigMapHash(release, configMapPath, "cilium-config")
	if err != nil {
		return nil, err
	}

	var newConfigMapMigrationHash string
	if values.Global.IsMigratingCNI {
		newConfigMapMigrationHash, err = getConfigMapHash(release, configMapPath, "cilium-config-migration")
		if err != nil {
			return nil, err
		}
	}

	configMapPath = "cilium/charts/config/templates/label-prefix-configmap.yaml"
	newConfigMapLabelPrefixHash, err := getConfigMapHash(release, configMapPath, "label-prefix-conf")
	if err != nil {
		return nil, err
	}

	if newConfigMapHash != configMapHash || newConfigMapLabelPrefixHash != configMapLabelPrefixHash || newConfigMapMigrationHash != values.Global.ConfigMapMigrationHash {
		// Render the charts with the new configMap hash.
		values, err := ComputeCiliumChartValues(config, network, cluster, ipamMode, newConfigMapHash, configMapLabelPrefixHash)
		if err != nil {
			return nil, err
		}
		values.Global.ConfigMapMigrationHash = newConfigMapMigrationHash

		release, err = renderer.RenderEmbeddedFS(charts.InternalChart, cilium.CiliumChartPath, cilium.ReleaseName, metav1.NamespaceSystem, values)
		if err != nil {
			return nil, err
		}
	}

	return release.Manifest(), nil
}

func getConfigMapHash(release *chartrenderer.RenderedChart, configMapPath, configMapName string) (string, error) {
	// Only hash the given configmap, the template file might contain multiple configmaps.
	resourceName := "configmap/" + configMapName
	configMapData, ok := release.Files()[configMapPath][resourceName]
	if !ok {
		return "", fmt.Errorf("configmap %s not found in the given path: %s", configMapName, configMapPath)
	}

	return utils.ComputeConfigMapChecksum(map[string]string{resourceName: configMapData}), nil
}
