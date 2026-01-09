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
	"errors"
	"fmt"
	"os"
	"path"
	"time"

	vmschema "kubevirt.io/api/core/v1"

	domainschema "kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/api"

	"kubevirt.io/client-go/log"

	"kubevirt.io/kubevirt/pkg/apimachinery/wait"
	"kubevirt.io/kubevirt/pkg/network/downwardapi"
	"kubevirt.io/kubevirt/pkg/virt-launcher/virtwrap/device"

	"kubevirt.io/kubevirt/pkg/network/vmispec"
)

type NetworkConfiguratorOptions struct {
	IstioProxyInjectionEnabled bool
	UseVirtioTransitional      bool
}

type VdpaNetworkConfigurator struct {
	vmiSpecIfaces []*vmschema.Interface
	options       NetworkConfiguratorOptions
	vdpaPaths     []string
	macAddrs      []string
}

const (
	// VdpaPluginName vdpa binding plugin name should be registered to Kubevirt through Kubevirt CR
	VdpaPluginName = "vdpa"
	// VdpaLogFilePath vdpa log file path Kubevirt consume and record
	VdpaLogFilePath = "/var/run/kubevirt/vdpa.log"
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

func getIfaceVdpaConfigurator(ifaces []*vmschema.Interface, opts NetworkConfiguratorOptions) (*VdpaNetworkConfigurator, error) {
	netInfo, err := getDownwardAPINetworkInfo()
	if err != nil {
		return nil, err
	}

	var macs []string
	var vdpaPaths []string

	for _, iface := range ifaces {
		for _, net := range netInfo.Interfaces {
			if net.Network == iface.Name {
				macs = append(macs, net.Mac)
				vdpaPaths = append(vdpaPaths, net.DeviceInfo.Vdpa.Path)
			}
		}
	}

	nIf := len(ifaces)

	if len(macs) != nIf || len(vdpaPaths) != nIf {
		return nil, fmt.Errorf("not all vdpa interfaces were found in NetworkInfo")
	}

	return &VdpaNetworkConfigurator{
		vmiSpecIfaces: ifaces,
		options:       opts,
		vdpaPaths:     vdpaPaths,
		macAddrs:      macs,
	}, nil

}

func NewVdpaNetworkConfigurator(ifaces []vmschema.Interface, networks []vmschema.Network, opts NetworkConfiguratorOptions) (*VdpaNetworkConfigurator, error) {

	var vdpaIfaces []*vmschema.Interface
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

			vdpaIfaces = append(vdpaIfaces, iface)
		}
	}

	if len(vdpaIfaces) == 0 {
		return nil, fmt.Errorf("no vdpa interface found")
	}

	return getIfaceVdpaConfigurator(vdpaIfaces, opts)
}

func (p VdpaNetworkConfigurator) Mutate(domainSpec *domainschema.DomainSpec) (*domainschema.DomainSpec, error) {
	generatedIfaces, err := p.generateInterfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to generate domain interface spec: %v", err)
	}

	domainSpecCopy := domainSpec.DeepCopy()
	for i, domainIface := range p.vmiSpecIfaces {
		if iface := lookupIfaceByAliasName(domainSpecCopy.Devices.Interfaces, domainIface.Name); iface != nil {
			*iface = *generatedIfaces[i]
		} else {
			domainSpecCopy.Devices.Interfaces = append(domainSpecCopy.Devices.Interfaces, *generatedIfaces[i])
		}

		log.Log.Infof("vdpa interface %s is added to domain spec successfully: %+v", domainIface.Name, generatedIfaces)
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
	var pciAddress *domainschema.Address
	var domainInterfaces []*domainschema.Interface

	for i, iface := range p.vmiSpecIfaces {
		if iface.PciAddress != "" {
			var err error
			pciAddress, err = device.NewPciAddressField(iface.PciAddress)
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
		if iface.MacAddress != "" {
			mac = &domainschema.MAC{MAC: iface.MacAddress}
		} else if p.macAddrs[i] != "" {
			mac = &domainschema.MAC{MAC: p.macAddrs[i]}
		}

		var acpi *domainschema.ACPI
		if iface.ACPIIndex > 0 {
			acpi = &domainschema.ACPI{Index: uint(iface.ACPIIndex)}
		}

		const (
			ifaceTypeUser = "vdpa"
			// ifaceBackendVdpa = "vdpa"
		)

		domainInterfaces = append(domainInterfaces, &domainschema.Interface{
			Alias:   domainschema.NewUserDefinedAlias(iface.Name),
			Model:   model,
			Address: pciAddress,
			MAC:     mac,
			ACPI:    acpi,
			Type:    ifaceTypeUser,
			Source:  domainschema.InterfaceSource{Device: p.vdpaPaths[i]},
			// PortForward: p.generatePortForward(),
		})
	}

	return domainInterfaces, nil
}
