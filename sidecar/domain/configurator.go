/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2023 Red Hat, Inc.
 *
 */

package domain

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"time"

	"kubevirt.io/client-go/log"

	vmschema "kubevirt.io/api/core/v1"
	"kubevirt.io/kubevirt/pkg/apimachinery/wait"
	"kubevirt.io/kubevirt/pkg/network/downwardapi"
	netnamescheme "kubevirt.io/kubevirt/pkg/network/namescheme"
	"kubevirt.io/kubevirt/pkg/network/vmispec"
	domainschema "kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/device"

	"kubevirt.io/vdpa-network-binding-plugin/sidecar/symlink"
)

type VdpaIfaceConfig struct {
	vmiSpecIface *vmschema.Interface
	vdpaPath     string
	symlinkName  string
	macAddr      string
}

type VdpaNetworkConfigurator struct {
	vdpaConfigs   []*VdpaIfaceConfig
	containerName string
}

const (
	// VdpaPluginName vdpa binding plugin name should be registered to Kubevirt through Kubevirt CR
	VdpaPluginName = "vdpa"
)

func readFileUntilNotEmpty(networkPCIMapPath string) ([]byte, error) {
	var networkPCIMapBytes []byte

	err := wait.PollImmediately(100*time.Millisecond, time.Second, func(_ context.Context) (bool, error) {
		var err error
		networkPCIMapBytes, err = os.ReadFile(networkPCIMapPath)
		return len(networkPCIMapBytes) > 0, err
	})

	return networkPCIMapBytes, err
}

func GetDownwardAPINetworkInfo(filePath string) (*downwardapi.NetworkInfo, error) {
	networkPCIMapBytes, err := readFileUntilNotEmpty(filePath)
	if err != nil {
		return nil, err
	}

	result := &downwardapi.NetworkInfo{}
	err = json.Unmarshal(networkPCIMapBytes, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func lookupNetworkInfoByName(name string, netInfo *downwardapi.NetworkInfo) (*downwardapi.Interface, error) {
	for _, ifaceInfo := range netInfo.Interfaces {
		if name == ifaceInfo.Network {
			return &ifaceInfo, nil
		}
	}
	return nil, fmt.Errorf("failed to find networkinfo for interface %s", name)
}

func NewVdpaNetworkConfigurator(
	ifaces []vmschema.Interface,
	networks []vmschema.Network,
	netInfo *downwardapi.NetworkInfo,
	containerName string,
) (*VdpaNetworkConfigurator, error) {
	netNameSchema := netnamescheme.CreateHashedNetworkNameScheme(networks)
	var configs []*VdpaIfaceConfig
	for _, net := range networks {
		if net.Multus != nil {
			iface := vmispec.LookupInterfaceByName(ifaces, net.Name)
			if iface == nil {
				return nil, fmt.Errorf("no interface named %s found", net.Name)
			}

			if iface.Binding == nil || iface.Binding.Name != VdpaPluginName {
				log.Log.Infof("interface %q is not set with Vdpa network binding plugin", net.Name)
				continue
			}

			ifaceInfo, err := lookupNetworkInfoByName(net.Name, netInfo)
			if err != nil {
				return nil, err
			}

			podNetName := netNameSchema[net.Name]

			configs = append(configs,
				&VdpaIfaceConfig{
					vmiSpecIface: iface,
					vdpaPath:     ifaceInfo.DeviceInfo.Vdpa.Path,
					symlinkName:  podNetName,
					macAddr:      ifaceInfo.Mac,
				},
			)
		}
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no vdpa interface found")
	}

	return &VdpaNetworkConfigurator{
			vdpaConfigs:   configs,
			containerName: containerName,
		},
		nil
}

func (p VdpaNetworkConfigurator) Mutate(domainSpec *domainschema.DomainSpec) (*domainschema.DomainSpec, error) {
	generatedIfaces, err := p.generateInterfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to generate domain interface spec: %v", err)
	}

	domainSpecCopy := domainSpec.DeepCopy()
	for i, config := range p.vdpaConfigs {
		if iface := lookupIfaceByAliasName(domainSpecCopy.Devices.Interfaces, config.vmiSpecIface.Name); iface != nil {
			*iface = *generatedIfaces[i]
		} else {
			domainSpecCopy.Devices.Interfaces = append(domainSpecCopy.Devices.Interfaces, *generatedIfaces[i])
		}

		ifaceInfo, _ := xml.Marshal(generatedIfaces[i])
		log.Log.Infof("vdpa interface %s is added to domain spec successfully: %s",
			config.vmiSpecIface.Name, string(ifaceInfo))
	}

	return domainSpecCopy, nil
}

func lookupIfaceByAliasName(ifaces []domainschema.Interface, name string) *domainschema.Interface {
	for i, iface := range ifaces {
		if iface.Alias != nil && iface.Alias.GetName() == name {
			return &ifaces[i]
		}
	}

	return nil
}

func (p VdpaNetworkConfigurator) generateInterfaces() ([]*domainschema.Interface, error) {
	var domainInterfaces []*domainschema.Interface

	for _, cfg := range p.vdpaConfigs {
		var pciAddress *domainschema.Address
		if cfg.vmiSpecIface.PciAddress != "" {
			var err error
			pciAddress, err = device.NewPciAddressField(cfg.vmiSpecIface.PciAddress)
			if err != nil {
				return nil, err
			}
		}

		var mac *domainschema.MAC
		if cfg.vmiSpecIface.MacAddress != "" {
			mac = &domainschema.MAC{MAC: cfg.vmiSpecIface.MacAddress}
		} else if cfg.macAddr != "" {
			mac = &domainschema.MAC{MAC: cfg.macAddr}
		}

		var acpi *domainschema.ACPI
		if cfg.vmiSpecIface.ACPIIndex > 0 {
			acpi = &domainschema.ACPI{Index: uint(cfg.vmiSpecIface.ACPIIndex)}
		}

		vdpaPath := symlink.SharedComputeSymlinkPath(p.containerName, cfg.symlinkName)

		domainInterfaces = append(domainInterfaces, &domainschema.Interface{
			Alias:   domainschema.NewUserDefinedAlias(cfg.vmiSpecIface.Name),
			Model:   &domainschema.Model{Type: "virtio"},
			Address: pciAddress,
			MAC:     mac,
			ACPI:    acpi,
			Type:    "vdpa",
			Source:  domainschema.InterfaceSource{Device: vdpaPath},
		})
	}

	return domainInterfaces, nil
}

func (p *VdpaNetworkConfigurator) VdpaPathsToSymlinkNames() map[string]string {
	pathsToSymlinks := make(map[string]string, len(p.vdpaConfigs))
	for _, cfg := range p.vdpaConfigs {
		pathsToSymlinks[cfg.vdpaPath] = cfg.symlinkName
	}
	return pathsToSymlinks
}
