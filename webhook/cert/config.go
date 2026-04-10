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

package cert

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"kubevirt.io/client-go/log"

	"kubevirt.io/kubevirt/pkg/apimachinery/patch"
)

func patchWebhookCABundle(clientset kubernetes.Interface, caCertPEM []byte, webhookName string) error {
	patchSet := patch.New(patch.WithAdd("/webhooks/0/clientConfig/caBundle", caCertPEM))
	patchBytes, err := patchSet.GeneratePayload()
	if err != nil {
		return fmt.Errorf("failed to generate patch: %v", err)
	}

	_, err = clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().Patch(context.Background(), webhookName, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch webhook caBundle: %v", err)
	}

	log.Log.V(2).Infof("Patched caBundle on MutatingWebhookConfiguration: %s", webhookName)
	return nil
}
