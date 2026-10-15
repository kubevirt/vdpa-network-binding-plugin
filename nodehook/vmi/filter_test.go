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
 */

package vmi_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"kubevirt.io/api/core/v1"
	"kubevirt.io/kubevirt/pkg/libvmi"

	filters "kubevirt.io/vdpa-network-binding-plugin/nodehook/vmi"
)

const testBindingPluginName = "matchingbindingplugin"

var _ = Describe("network binding plugin vmi filter", func() {
	DescribeTable("selects the vmis that match the criteria",
		func(vmi *v1.VirtualMachineInstance, expectedMatch bool) {
			filter := filters.NewNBPVMIFilter(testBindingPluginName)
			Expect(filter.Matches(vmi)).To(Equal(expectedMatch))
		},
		Entry("without network binding plugins attached",
			libvmi.New(),
			false,
		),
		Entry("vmis with other network binding plugins",
			libvmi.New(
				libvmi.WithInterface(
					libvmi.InterfaceWithBindingPlugin(
						"not-with-our-binding",
						v1.PluginBinding{Name: "other-sort-of-binding"},
					),
				),
				libvmi.WithNetwork(
					libvmi.MultusNetwork("not-with-our-binding", "nad"),
				),
			),
			false,
		),
		Entry("vmis with expected network binding plugins",
			libvmi.New(
				libvmi.WithInterface(
					libvmi.InterfaceWithBindingPlugin(
						"with-our-binding",
						v1.PluginBinding{Name: testBindingPluginName},
					),
				),
				libvmi.WithNetwork(
					libvmi.MultusNetwork("with-our-binding", "nad"),
				),
			),
			true,
		),
		Entry("vmis with expected network binding plugins but no matching network",
			libvmi.New(
				libvmi.WithInterface(
					libvmi.InterfaceWithBindingPlugin(
						"not-with-our-binding",
						v1.PluginBinding{Name: "other-sort-of-binding"},
					),
				),
				libvmi.WithNetwork(
					libvmi.MultusNetwork("not-with-our-binding", "nad-other-binding"),
				),
				libvmi.WithInterface(
					libvmi.InterfaceWithBindingPlugin(
						"with-our-binding",
						v1.PluginBinding{Name: testBindingPluginName},
					),
				),
				libvmi.WithNetwork(
					libvmi.MultusNetwork("non-matching-newtork", "nad-non-matching"),
				),
			),
			false,
		),
	)
})
