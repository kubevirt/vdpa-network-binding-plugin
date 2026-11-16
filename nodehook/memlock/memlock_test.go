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

package memlock

import (
	"fmt"

	"golang.org/x/sys/unix"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/kubevirt/pkg/libvmi"

	"kubevirt.io/vdpa-network-binding-plugin/nodehook/process"
)

var _ = Describe("Memory lock RLimits configurator", func() {
	var (
		ctrl     *gomock.Controller
		finder   *process.MockVmiProcessFinder
		vmi      *v1.VirtualMachineInstance
		callPid  int
		callSize uint64
		retErr   error
	)

	mockSetProcessMemlockRLimits := func(pid int, size uint64) error {
		callPid = pid
		callSize = size
		return retErr
	}

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		finder = process.NewMockVmiProcessFinder(ctrl)
		vmi = libvmi.New()
		callPid = 0
		callSize = 0
		retErr = nil
		setProcessMemlockRLimitsFunc = mockSetProcessMemlockRLimits
	})

	It("handles errors that might occur finding processes under the virt-launcher", func() {
		finder.EXPECT().Find(vmi).Return(nil, fmt.Errorf("error dealing with the virt-launcher"))
		res := ConfigureMemlockRLimits(vmi, finder)
		Expect(res).NotTo(BeNil())
	})

	It("returns an error if the target process was not found under virt-launcher", func() {
		finderInfoString := "process that the mock finder should have found"
		finder.EXPECT().Find(vmi).Return(nil, nil)
		finder.EXPECT().Info().Return(finderInfoString)
		res := ConfigureMemlockRLimits(vmi, finder)
		Expect(res).To(Equal(fmt.Errorf("did not find %s for vmi %s", finderInfoString, vmi.Name)))
	})

	It("returns an error if an error occurred setting the memlock RLimit", func() {
		mockPid := 8

		finder.EXPECT().Find(vmi).Return(&mockPid, nil)
		retErr = fmt.Errorf("problems setting memlock RLimits")

		res := ConfigureMemlockRLimits(vmi, finder)
		Expect(callPid).To(Equal(mockPid))
		Expect(callSize).To(Equal(uint64(unix.RLIM_INFINITY)))
		Expect(res).To(Equal(retErr))
	})

	It("sets an unlimited memlock RLimit to the right process", func() {
		mockPid := 123

		finder.EXPECT().Find(vmi).Return(&mockPid, nil)

		res := ConfigureMemlockRLimits(vmi, finder)
		Expect(callPid).To(Equal(mockPid))
		Expect(callSize).To(Equal(uint64(unix.RLIM_INFINITY)))
		Expect(res).To(BeNil())
	})
})
