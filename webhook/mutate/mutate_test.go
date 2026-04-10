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
 * Copyright The KubeVirt Authors.
 *
 */

package mutate

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admission/v1"
	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"

	"kubevirt.io/kubevirt/pkg/apimachinery/patch"

	v1 "kubevirt.io/api/core/v1"
)

func newAdmissionReviewFromVM(vm *v1.VirtualMachine) *admissionv1.AdmissionReview {
	vmBytes, err := json.Marshal(vm)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	By("Creating the test admissions review from the VM")
	return &admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			Namespace: "default",
			Name:      vm.Name,
			Object:    runtime.RawExtension{Raw: vmBytes},
		},
	}
}

func getPatchesFromResponse(resp *admissionv1.AdmissionResponse) []patch.PatchOperation {
	ExpectWithOffset(1, resp).ToNot(BeNil())
	ExpectWithOffset(1, resp.Allowed).To(BeTrue())
	if resp.Patch == nil {
		return nil
	}
	patches, err := patch.UnmarshalPatch(resp.Patch)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return patches
}

func findPatch(patches []patch.PatchOperation, path string) *patch.PatchOperation {
	for i := range patches {
		if patches[i].Path == path {
			return &patches[i]
		}
	}
	return nil
}

var _ = Describe("vDPA Mutating Webhook", func() {
	var vm *v1.VirtualMachine

	BeforeEach(func() {
		guestMemory := resource.MustParse("2Gi")
		vm = &v1.VirtualMachine{
			Spec: v1.VirtualMachineSpec{
				Template: &v1.VirtualMachineInstanceTemplateSpec{
					Spec: v1.VirtualMachineInstanceSpec{
						Domain: v1.DomainSpec{
							Memory: &v1.Memory{
								Guest: &guestMemory,
							},
						},
					},
				},
			},
		}
	})

	Context("mutateVM", func() {
		It("should not mutate VM when no vDPA interfaces are present", func() {
			vm.Spec.Template.Spec.Domain.Devices.Interfaces = []v1.Interface{
				{Name: "masq-net", InterfaceBindingMethod: v1.InterfaceBindingMethod{Masquerade: &v1.InterfaceMasquerade{}}},
			}
			ar := newAdmissionReviewFromVM(vm)
			resp := mutateVM(ar)
			Expect(resp.Allowed).To(BeTrue())
			Expect(resp.Patch).To(BeNil())
		})

		It("should set AddedOverhead=1Gi and MemLock=Required for a single vDPA interface", func() {
			vm.Spec.Template.Spec.Domain.Devices.Interfaces = []v1.Interface{
				{Name: "vdpa-net", Binding: &v1.PluginBinding{Name: "vdpa"}},
			}
			ar := newAdmissionReviewFromVM(vm)
			resp := mutateVM(ar)
			patches := getPatchesFromResponse(resp)

			memLockPatch := findPatch(patches, pathMemLock)
			Expect(memLockPatch).ToNot(BeNil())
			Expect(memLockPatch.Value).To(Equal(string(v1.MemLockRequired)))

			overheadPatch := findPatch(patches, pathAddedOverhead)
			Expect(overheadPatch).ToNot(BeNil())
			Expect(overheadPatch.Value).To(Equal("1Gi"))
		})

		It("should calculate correct overhead for multiple vDPA interfaces", func() {
			vm.Spec.Template.Spec.Domain.Devices.Interfaces = []v1.Interface{
				{Name: "vdpa-net1", Binding: &v1.PluginBinding{Name: "vdpa"}},
				{Name: "vdpa-net2", Binding: &v1.PluginBinding{Name: "vdpa"}},
			}
			ar := newAdmissionReviewFromVM(vm)
			resp := mutateVM(ar)
			patches := getPatchesFromResponse(resp)

			overheadPatch := findPatch(patches, pathAddedOverhead)
			Expect(overheadPatch).ToNot(BeNil())
			Expect(overheadPatch.Value).To(Equal("3Gi")) // 1Gi + (2-1)*2Gi = 3Gi
		})

		It("should add user-defined overhead on top of vDPA calculated overhead", func() {
			userOverhead := resource.MustParse("512Mi")
			vm.Spec.Template.Spec.Domain.Memory.ReservedOverhead = &v1.ReservedOverhead{
				AddedOverhead: &userOverhead,
			}
			vm.Spec.Template.Spec.Domain.Devices.Interfaces = []v1.Interface{
				{Name: "vdpa-net", Binding: &v1.PluginBinding{Name: "vdpa"}},
			}
			ar := newAdmissionReviewFromVM(vm)
			resp := mutateVM(ar)
			patches := getPatchesFromResponse(resp)

			overheadPatch := findPatch(patches, pathAddedOverhead)
			Expect(overheadPatch).ToNot(BeNil())
			result := resource.MustParse(overheadPatch.Value.(string))
			expected := resource.MustParse("1536Mi") // 1Gi + 512Mi
			Expect(result.Cmp(expected)).To(BeZero())
		})

		It("should count only vDPA interfaces when mixed with non-vDPA", func() {
			vm.Spec.Template.Spec.Domain.Devices.Interfaces = []v1.Interface{
				{Name: "masq-net", InterfaceBindingMethod: v1.InterfaceBindingMethod{Masquerade: &v1.InterfaceMasquerade{}}},
				{Name: "vdpa-net", Binding: &v1.PluginBinding{Name: "vdpa"}},
				{Name: "passt-net", Binding: &v1.PluginBinding{Name: "passt"}},
			}
			ar := newAdmissionReviewFromVM(vm)
			resp := mutateVM(ar)
			patches := getPatchesFromResponse(resp)

			overheadPatch := findPatch(patches, pathAddedOverhead)
			Expect(overheadPatch).ToNot(BeNil())
			Expect(overheadPatch.Value).To(Equal("1Gi"))
		})

		It("should calculate AddedOverhead using resources.requests.memory when guest memory is unset", func() {
			vm.Spec.Template.Spec.Domain.Memory = &v1.Memory{}
			vm.Spec.Template.Spec.Domain.Resources.Requests = map[k8sv1.ResourceName]resource.Quantity{
				k8sv1.ResourceMemory: resource.MustParse("4Gi"),
			}
			vm.Spec.Template.Spec.Domain.Devices.Interfaces = []v1.Interface{
				{Name: "vdpa-net1", Binding: &v1.PluginBinding{Name: "vdpa"}},
				{Name: "vdpa-net2", Binding: &v1.PluginBinding{Name: "vdpa"}},
			}
			ar := newAdmissionReviewFromVM(vm)
			resp := mutateVM(ar)
			patches := getPatchesFromResponse(resp)

			overheadPatch := findPatch(patches, pathAddedOverhead)
			Expect(overheadPatch).ToNot(BeNil())
			Expect(overheadPatch.Value).To(Equal("5Gi")) // 1Gi + (2-1)*4Gi = 5Gi
		})
	})
})
