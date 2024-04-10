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

	v1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"kubevirt.io/kubevirt/pkg/network/vmispec"
)

type Interface struct {
	Network    string         `json:"network"`
	DeviceInfo *v1.DeviceInfo `json:"deviceInfo,omitempty"`
}

type NetworkInfo struct {
	Interfaces []Interface `json:"interfaces,omitempty"`
}

type NetworkConfiguratorOptions struct {
	IstioProxyInjectionEnabled bool
	UseVirtioTransitional      bool
}

type VdpaNetworkConfigurator struct {
	vmiSpecIface *vmschema.Interface
	options      NetworkConfiguratorOptions
	vdpaPath     string
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

func getVdpaPath(path string) (string, error) {
	networkPCIMapBytes, err := readFileUntilNotEmpty(path)
	if err != nil {
		if isFileEmptyAfterTimeout(err, networkPCIMapBytes) {
			return "", err
		}
		return "", nil
	}

	var result NetworkInfo
	err = json.Unmarshal(networkPCIMapBytes, &result)
	if err != nil {
		return "", err
	}

	return result.Interfaces[0].DeviceInfo.Vdpa.Path, nil
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

	netStatusPath := path.Join(downwardapi.MountPath, downwardapi.NetworkInfoVolumePath)
	vdpaPath, err := getVdpaPath(netStatusPath)
	if err != nil {
		return nil, err
	}

	iface := vmispec.LookupInterfaceByName(ifaces, network.Name)
	if iface == nil {
		return nil, fmt.Errorf("no interface found")
	}
	if iface.Binding == nil || iface.Binding != nil && iface.Binding.Name != VdpaPluginName {
		return nil, fmt.Errorf("interface %q is not set with Vdpa network binding plugin", network.Name)
	}

	return &VdpaNetworkConfigurator{
		vmiSpecIface: iface,
		options:      opts,
		vdpaPath:     vdpaPath,
	}, nil
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
