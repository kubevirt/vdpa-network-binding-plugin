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

package integration

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
	"kubevirt.io/kubevirt/pkg/libvmi"
	"kubevirt.io/kubevirt/tests/console"
	"kubevirt.io/kubevirt/tests/exec"
	"kubevirt.io/kubevirt/tests/flags"
	"kubevirt.io/kubevirt/tests/libmigration"
	"kubevirt.io/kubevirt/tests/libpod"
	"kubevirt.io/kubevirt/tests/libvmifact"
	"kubevirt.io/kubevirt/tests/libvmops"
)

var _ = Describe("memlock node hook plugin", func() {
	var vmiName string
	var vmi *v1.VirtualMachineInstance
	var virClient kubecli.KubevirtClient
	var err error

	// Defaults to 8MB
	const defaultMemoryLockLimit string = "8388608"
	const defaultMemoryLockLimitUnit string = "bytes"

	flags.NormalizeFlags()

	verifyDefaultMemlockLimits := func() {
		By("Getting the virt-launcher pod")
		pod, err := libpod.GetPodByVirtualMachineInstance(vmi, vmi.Namespace)
		Expect(err).To(BeNil())

		By("Retrieving virt-launcher process limits")
		output, err := exec.ExecuteCommandOnPod(pod, "compute", []string{
			"find", "/proc", "-type", "d", "-name", "[0-9]*", "-maxdepth", "1",
			"-exec", "cat", "{}/limits", ";",
		})
		Expect(err).To(BeNil())

		By("Checking that memlock RLimits are set to defaults for all processes")
		memLockLimitsChecked := false
		for _, line := range strings.Split(output, "\n") {
			cleanLine, isMemLockLine := strings.CutPrefix(line, "Max locked memory")
			if isMemLockLine {
				fields := strings.Fields(cleanLine)
				Expect(len(fields)).To(Equal(3), fmt.Sprintf("unexpected format for Max locked memory: %q", line))

				Expect(fields[0]).To(Equal(defaultMemoryLockLimit), "unexpected soft memory lock limit")
				Expect(fields[1]).To(Equal(defaultMemoryLockLimit), "unexpected hard memory lock limit")
				Expect(fields[2]).To(Equal(defaultMemoryLockLimitUnit), "unexpected hard memory lock limit unit")

				memLockLimitsChecked = true
			}
		}
		Expect(memLockLimitsChecked).To(BeTrue(), "memlock limits could not be checked properly")
	}

	BeforeEach(func() {
		vmiName = RandomVMIName()
		virClient, err = kubecli.GetKubevirtClient()
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		deleteVMIAndWait(vmi, virClient)
	})

	It("does not increase memlock limits of VMIs without vdpa interfaces", func() {
		By("run VMI without VDPA interfaces")
		vmi = libvmifact.NewAlpine(
			libvmi.WithNamespace(vdpaTestNamespace),
			libvmi.WithName(vmiName),
			libvmi.WithInterface(libvmi.InterfaceDeviceWithMasqueradeBinding()),
			libvmi.WithNetwork(v1.DefaultPodNetwork()),
		)
		vmi = libvmops.RunVMIAndExpectLaunch(vmi, libvmops.StartupTimeoutSecondsSmall)

		By("logging in")
		Expect(console.LoginToAlpine(vmi)).To(Succeed())

		verifyDefaultMemlockLimits()

		By("starting the migration")
		migration := libmigration.New(vmi.Name, vmi.Namespace)
		migration = libmigration.RunMigrationAndExpectToCompleteWithDefaultTimeout(virClient, migration)
		libmigration.ConfirmVMIPostMigration(virClient, vmi, migration)

		By("logging in dst")
		Expect(console.LoginToAlpine(vmi)).To(Succeed())

		verifyDefaultMemlockLimits()
	})
})
