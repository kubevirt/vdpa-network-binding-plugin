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
	"fmt"
	"io"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"kubevirt.io/kubevirt/pkg/apimachinery/patch"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/log"
)

const (
	vdpaBindingName      = "vdpa"
	pathMemory           = "/spec/template/spec/domain/memory"
	pathReservedOverhead = pathMemory + "/reservedOverhead"
	pathMemLock          = pathReservedOverhead + "/memLock"
	pathAddedOverhead    = pathReservedOverhead + "/addedOverhead"
)

func HandleMutateVDPA(resp http.ResponseWriter, req *http.Request) {
	review, err := getAdmissionReview(req)
	if err != nil {
		log.Log.Reason(err).Error("failed to parse AdmissionReview")
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	response := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: admissionv1.SchemeGroupVersion.String(),
			Kind:       "AdmissionReview",
		},
	}

	reviewResponse := mutateVM(review)
	if reviewResponse != nil {
		response.Response = reviewResponse
		response.Response.UID = review.Request.UID
	}
	// reset the Object and OldObject, they are not needed in a response.
	review.Request.Object = runtime.RawExtension{}
	review.Request.OldObject = runtime.RawExtension{}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		log.Log.Reason(err).Errorf("failed json encode webhook response")
		resp.WriteHeader(http.StatusBadRequest)
		return
	}

	resp.Header().Set("Content-Type", "application/json")
	if _, err := resp.Write(responseBytes); err != nil {
		log.Log.Reason(err).Errorf("failed to write webhook response: %v", err)
	}
}

func mutateVM(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	vm := &v1.VirtualMachine{}
	if err := json.Unmarshal(ar.Request.Object.Raw, vm); err != nil {
		log.Log.Reason(err).Error("Failed to unmarshal VM from AdmissionReview")
		return allowWithoutPatch()
	}

	if vm.Spec.Template == nil {
		return allowWithoutPatch()
	}

	vmSpec := &vm.Spec.Template.Spec
	vdpaCount := countVDPAInterfaces(vmSpec.Domain.Devices.Interfaces)
	if vdpaCount == 0 {
		log.Log.V(4).Infof("VM %s/%s has no vDPA interfaces, skipping mutation", ar.Request.Namespace, ar.Request.Name)
		return allowWithoutPatch()
	}

	requiredOverhead, err := calculateRequiredVDPAMemoryOverhead(vdpaCount, vmSpec)
	if err != nil {
		log.Log.Warningf("Cannot calculate vDPA overhead for VM %s/%s: %v", ar.Request.Namespace, ar.Request.Name, err)
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Message: fmt.Sprintf("failed to calculate vDPA memory overhead: %v", err),
			},
		}
	}

	patchSet := patch.New()
	memoryExists := vmSpec.Domain.Memory != nil
	reservedOverheadExists := memoryExists && vmSpec.Domain.Memory.ReservedOverhead != nil

	if !memoryExists {
		patchSet.AddOption(patch.WithAdd(pathMemory, map[string]interface{}{}))
	}
	if !reservedOverheadExists {
		patchSet.AddOption(patch.WithAdd(pathReservedOverhead, map[string]interface{}{}))
	}

	if !(reservedOverheadExists && vmSpec.Domain.Memory.ReservedOverhead.MemLock != nil && *vmSpec.Domain.Memory.ReservedOverhead.MemLock == v1.MemLockRequired) {
		log.Log.V(2).Infof("Setting MemLock to Required for VM %s/%s", ar.Request.Namespace, ar.Request.Name)
		patchSet.AddOption(patch.WithAdd(pathMemLock, v1.MemLockRequired))
	}

	totalOverhead := requiredOverhead.DeepCopy()
	if reservedOverheadExists && vmSpec.Domain.Memory.ReservedOverhead.AddedOverhead != nil {
		totalOverhead.Add(*vmSpec.Domain.Memory.ReservedOverhead.AddedOverhead)
	}
	log.Log.V(2).Infof("Setting vDPA AddedOverhead to %s for VM %s/%s (%d vDPA interfaces, calculated: %s)", totalOverhead.String(), ar.Request.Namespace, ar.Request.Name, vdpaCount, requiredOverhead.String())
	patchSet.AddOption(patch.WithAdd(pathAddedOverhead, totalOverhead.String()))

	patchBytes, err := patchSet.GeneratePayload()
	if err != nil {
		log.Log.Reason(err).Error("Failed to marshal JSON patches")
		return allowWithoutPatch()
	}

	patchType := admissionv1.PatchTypeJSONPatch
	return &admissionv1.AdmissionResponse{
		Allowed:   true,
		Patch:     patchBytes,
		PatchType: &patchType,
	}
}

func countVDPAInterfaces(ifaces []v1.Interface) int {
	count := 0
	for _, iface := range ifaces {
		if iface.Binding != nil && iface.Binding.Name == vdpaBindingName {
			count++
		}
	}
	return count
}

// calculateRequiredVDPAMemoryOverhead computes the minimum AddedOverhead for vDPA.
// Formula: 1Gi + (N-1) * guest_memory for N vDPA interfaces.
func calculateRequiredVDPAMemoryOverhead(vdpaCount int, vmSpec *v1.VirtualMachineInstanceSpec) (*resource.Quantity, error) {
	if vdpaCount <= 0 {
		return nil, fmt.Errorf("invalid vdpa interface count: %d", vdpaCount)
	}
	requiredAddedOverhead := resource.MustParse("1Gi")
	if vdpaCount == 1 {
		return &requiredAddedOverhead, nil
	}
	guestMemory := getGuestMemoryQuantity(vmSpec)
	if guestMemory == nil {
		return nil, fmt.Errorf("guest memory not specified, can't calculate required overhead")
	}

	additionalOverhead := resource.NewQuantity(guestMemory.Value()*int64(vdpaCount-1), guestMemory.Format)
	requiredAddedOverhead.Add(*additionalOverhead)
	return &requiredAddedOverhead, nil
}

func getGuestMemoryQuantity(vmSpec *v1.VirtualMachineInstanceSpec) *resource.Quantity {
	if vmSpec == nil {
		return nil
	}

	if vmSpec.Domain.Memory != nil && vmSpec.Domain.Memory.Guest != nil && !vmSpec.Domain.Memory.Guest.IsZero() {
		return vmSpec.Domain.Memory.Guest
	}

	if memReq := vmSpec.Domain.Resources.Requests.Memory(); memReq != nil && !memReq.IsZero() {
		return memReq
	}

	return nil
}

func getAdmissionReview(req *http.Request) (*admissionv1.AdmissionReview, error) {
	var body []byte
	if req.Body != nil {
		if data, err := io.ReadAll(req.Body); err == nil {
			body = data
		}
	}
	contentType := req.Header.Get("Content-Type")
	if contentType != "application/json" {
		return nil, fmt.Errorf("expected content-type application/json, got %s", contentType)
	}

	ar := &admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, ar); err != nil {
		return nil, fmt.Errorf("failed to unmarshal AdmissionReview: %w", err)
	}
	return ar, nil
}

func allowWithoutPatch() *admissionv1.AdmissionResponse {
	return &admissionv1.AdmissionResponse{Allowed: true}
}
