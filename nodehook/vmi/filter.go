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
 *
 */

package vmi

//go:generate mockgen -source $GOFILE -package=$GOPACKAGE -destination=generated_mock_$GOFILE

import (
	v1 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/network/vmispec"
)

type VMIFilter interface {
	// Returns true if a given vmi matches a set of conditions
	Matches(vmi *v1.VirtualMachineInstance) bool
}

// NBP stands for Network Binding Plugin
// It checks if the vmi contains a network binding plugin with a
// matching name
type NBPVMIFilter struct {
	netBindingName string
}

func NewNBPVMIFilter(name string) *NBPVMIFilter {
	return &NBPVMIFilter{name}
}

func (f *NBPVMIFilter) Matches(vmi *v1.VirtualMachineInstance) bool {
	ifaces := vmi.Spec.Domain.Devices.Interfaces
	for _, net := range vmi.Spec.Networks {
		if net.Multus != nil {
			iface := vmispec.LookupInterfaceByName(ifaces, net.Name)
			if iface != nil &&
				iface.Binding != nil &&
				iface.Binding.Name == f.netBindingName {
				return true
			}
		}
	}

	return false
}
