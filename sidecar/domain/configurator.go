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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	vmschema "kubevirt.io/api/core/v1"

	domainschema "kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"

	"kubevirt.io/client-go/log"

	"kubevirt.io/kubevirt/pkg/network/downwardapi"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/device"

	"kubevirt.io/kubevirt/pkg/network/vmispec"
)

type NetworkConfiguratorOptions struct {
	IstioProxyInjectionEnabled bool
	UseVirtioTransitional      bool
}

type VdpaNetworkConfigurator struct {
	vmiSpecIface *vmschema.Interface
	options      NetworkConfiguratorOptions
	vdpaPath     string
	macAddr      string
}

const (
	// VdpaPluginName vdpa binding plugin name should be registered to Kubevirt through Kubevirt CR
	VdpaPluginName = "vdpa"
	// VdpaLogFilePath vdpa log file path Kubevirt consume and record
	VdpaLogFilePath = "/var/run/kubevirt/vdpa.log"
)

func readFileUntilNotEmpty(networkPCIMapPath string) ([]byte, error) {
	var networkPCIMapBytes []byte
	err := wait.PollImmediate(100*time.Millisecond, time.Second, func() (bool, error) {
		var err error
		networkPCIMapBytes, err = os.ReadFile(networkPCIMapPath)
		return len(networkPCIMapBytes) > 0, err
	})
	return networkPCIMapBytes, err
}

func isFileEmptyAfterTimeout(err error, data []byte) bool {
	return errors.Is(err, wait.ErrWaitTimeout) && len(data) == 0
}

func getDownwardAPINetworkInfo() (*downwardapi.NetworkInfo, error) {
	netStatusPath := path.Join(downwardapi.MountPath, downwardapi.NetworkInfoVolumePath)

	networkPCIMapBytes, err := readFileUntilNotEmpty(netStatusPath)
	if err != nil {
		if isFileEmptyAfterTimeout(err, networkPCIMapBytes) {
			return nil, err
		}
		return nil, nil
	}

	result := &downwardapi.NetworkInfo{}
	err = json.Unmarshal(networkPCIMapBytes, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func getIfaceVdpaConfigurator(iface *vmschema.Interface, opts NetworkConfiguratorOptions) (*VdpaNetworkConfigurator, error) {
	netInfo, err := getDownwardAPINetworkInfo()
	if err != nil {
		return nil, err
	}

	for _, net := range netInfo.Interfaces {
		if net.Network == iface.Name {
			return &VdpaNetworkConfigurator{
				vmiSpecIface: iface,
				options:      opts,
				vdpaPath:     net.DeviceInfo.Vdpa.Path,
				macAddr:      net.MacAddress,
			}, nil
		}
	}

	return nil, fmt.Errorf("interface %s not found in NetworkInfo", iface.Name)
}

func NewVdpaNetworkConfigurator(ifaces []vmschema.Interface, networks []vmschema.Network, opts NetworkConfiguratorOptions, deviceInfo string) (*VdpaNetworkConfigurator, error) {

	var network *vmschema.Network
	for _, net := range networks {
		if net.Multus != nil {
			network = &net

			break
		}
	}

	if network == nil {
		return nil, fmt.Errorf("multus network not found")
	}

	iface := vmispec.LookupInterfaceByName(ifaces, network.Name)
	if iface == nil {
		return nil, fmt.Errorf("no interface found")
	}
	if iface.Binding == nil || iface.Binding != nil && iface.Binding.Name != VdpaPluginName {
		return nil, fmt.Errorf("interface %q is not set with Vdpa network binding plugin", network.Name)
	}

	return getIfaceVdpaConfigurator(iface, opts)
}

func (p VdpaNetworkConfigurator) Mutate(domainSpec *domainschema.DomainSpec) (*domainschema.DomainSpec, error) {
	generatedIface, err := p.generateInterface()
	if err != nil {
		return nil, fmt.Errorf("failed to generate domain interface spec: %v", err)
	}

	domainSpecCopy := domainSpec.DeepCopy()
	if iface := lookupIfaceByAliasName(domainSpecCopy.Devices.Interfaces, p.vmiSpecIface.Name); iface != nil {
		*iface = *generatedIface
	} else {
		domainSpecCopy.Devices.Interfaces = append(domainSpecCopy.Devices.Interfaces, *generatedIface)
	}

	log.Log.Infof("vdpa interface is added to domain spec successfully: %+v", generatedIface)

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

func (p VdpaNetworkConfigurator) generateInterface() (*domainschema.Interface, error) {
	var pciAddress *domainschema.Address
	if p.vmiSpecIface.PciAddress != "" {
		var err error
		pciAddress, err = device.NewPciAddressField(p.vmiSpecIface.PciAddress)
		if err != nil {
			return nil, err
		}
	}

	/*
		var ifaceModel string
		if p.vmiSpecIface.Model == "" {
			ifaceModel = vmschema.VirtIO
		} else {
			ifaceModel = p.vmiSpecIface.Model
		}
		ifaceModel := "virtio"
	*/

	ifaceModelType := "virtio"
	/*
		var ifaceModelType string
		if ifaceModel == vmschema.VirtIO {
			if p.options.UseVirtioTransitional {
				ifaceModelType = "virtio-transitional"
			} else {
				ifaceModelType = "virtio-non-transitional"
			}
		} else {
			ifaceModelType = p.vmiSpecIface.Model
		}
	*/
	model := &domainschema.Model{Type: ifaceModelType}

	var mac *domainschema.MAC
	if p.vmiSpecIface.MacAddress != "" {
		mac = &domainschema.MAC{MAC: p.vmiSpecIface.MacAddress}
	} else if p.macAddr != "" {
		mac = &domainschema.MAC{MAC: p.macAddr}
	}

	var acpi *domainschema.ACPI
	if p.vmiSpecIface.ACPIIndex > 0 {
		acpi = &domainschema.ACPI{Index: uint(p.vmiSpecIface.ACPIIndex)}
	}

	const (
		ifaceTypeUser = "vdpa"
		// ifaceBackendVdpa = "vdpa"
	)

	return &domainschema.Interface{
		Alias:   domainschema.NewUserDefinedAlias(p.vmiSpecIface.Name),
		Model:   model,
		Address: pciAddress,
		MAC:     mac,
		ACPI:    acpi,
		Type:    ifaceTypeUser,
		Source:  domainschema.InterfaceSource{Device: p.vdpaPath},
		// PortForward: p.generatePortForward(),
	}, nil
}
