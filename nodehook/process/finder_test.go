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

package process

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"kubevirt.io/kubevirt/pkg/libvmi"
	"kubevirt.io/kubevirt/pkg/virt-handler/isolation"
)

var mockProcesses = []*ProcessInfo{
	{12, "notspyware"},
	{123, "virtqemud"},
	{1234, "qemu-kvm"},
	{12345, "someotherproc"},
}

func listMockProcesses(_ int) ([]*ProcessInfo, error) {
	return mockProcesses, nil
}

func errorWhileChildProcessRetrieval(_ int) ([]*ProcessInfo, error) {
	return nil, fmt.Errorf("this simulates an error while retrieving a process' child processes")
}

var _ = Describe("finding child processes of the virt-launcher pod", func() {
	var (
		ctrl            *gomock.Controller
		detector        *isolation.MockPodIsolationDetector
		isolationResult *isolation.MockIsolationResult
		filter          *MockProcessFilter
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		detector = isolation.NewMockPodIsolationDetector(ctrl)
		isolationResult = isolation.NewMockIsolationResult(ctrl)
		filter = NewMockProcessFilter(ctrl)
	})

	It("handles virt launcher detector errors", func() {
		vmi := libvmi.New()

		finder := &VirtLauncherProcessFinder{
			detector,
			listMockProcesses,
			filter,
		}
		detector.EXPECT().Detect(vmi).Return(nil, fmt.Errorf("virt launcher not found"))

		res, err := finder.Find(vmi)
		Expect(res).To(BeNil())
		Expect(err).NotTo(BeNil())
	})

	It("handles unexpected virt launcher returns", func() {
		vmi := libvmi.New()

		finder := &VirtLauncherProcessFinder{
			detector,
			listMockProcesses,
			filter,
		}
		detector.EXPECT().Detect(vmi).Return(nil, nil)

		res, err := finder.Find(vmi)
		Expect(res).To(BeNil())
		Expect(err).NotTo(BeNil())
	})

	It("handles when virt launcher processes can't be listed", func() {
		vmi := libvmi.New()

		finder := &VirtLauncherProcessFinder{
			detector,
			errorWhileChildProcessRetrieval,
			filter,
		}
		detector.EXPECT().Detect(vmi).Return(isolationResult, nil)
		isolationResult.EXPECT().Pid().Return(1)

		res, err := finder.Find(vmi)
		Expect(res).To(BeNil())
		Expect(err).NotTo(BeNil())
	})

	// wanted process is not under the processes that were running in the virt launcher
	It("returns nil when errors did not occur yet a matching process was not found", func() {
		vmi := libvmi.New()

		finder := &VirtLauncherProcessFinder{
			detector,
			listMockProcesses,
			filter,
		}
		detector.EXPECT().Detect(vmi).Return(isolationResult, nil)
		isolationResult.EXPECT().Pid().Return(1)
		for _, pInfo := range mockProcesses {
			filter.EXPECT().Matches(pInfo).Return(false)
		}
		filter.EXPECT().Info().Return("")

		res, err := finder.Find(vmi)
		Expect(res).To(BeNil())
		Expect(err).To(BeNil())
	})

	// process there is something yes :)
	It("returns the matching process' Pid", func() {
		vmi := libvmi.New()

		finder := &VirtLauncherProcessFinder{
			detector,
			listMockProcesses,
			filter,
		}
		detector.EXPECT().Detect(vmi).Return(isolationResult, nil)
		isolationResult.EXPECT().Pid().Return(1)
		for _, pInfo := range mockProcesses {
			if pInfo.ExecName == "qemu-kvm" {
				filter.EXPECT().Matches(pInfo).Return(true)
				break
			}
			filter.EXPECT().Matches(pInfo).Return(false)
		}

		res, err := finder.Find(vmi)
		Expect(res).NotTo(BeNil())
		Expect(*res).To(Equal(1234))
		Expect(err).To(BeNil())
	})
})
