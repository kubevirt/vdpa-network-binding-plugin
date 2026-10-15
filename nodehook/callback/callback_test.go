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

package callback

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/kubevirt/pkg/libvmi"

	"github.com/kubevirt/plugins/pkg/sdk/plugin"

	"kubevirt.io/vdpa-network-binding-plugin/nodehook/process"
	"kubevirt.io/vdpa-network-binding-plugin/nodehook/vmi"
)

var callbackTestNodeName string = "callbackUnitTestNodeName"

var _ = Describe("Node hook callback", func() {
	var (
		ctrl                     *gomock.Controller
		vmiFilter                *vmi.MockVMIFilter
		processFinder            *process.MockVmiProcessFinder
		testVmi                  *v1.VirtualMachineInstance
		memLockCallVmi           *v1.VirtualMachineInstance
		memLockCallFinder        process.VmiProcessFinder
		memlockConfigReturnError error
		ctx                      context.Context
	)

	mockMemlockConfigurator := func(
		vmi *v1.VirtualMachineInstance,
		processFinder process.VmiProcessFinder,
	) error {
		memLockCallVmi = testVmi
		memLockCallFinder = processFinder
		return memlockConfigReturnError
	}

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		vmiFilter = vmi.NewMockVMIFilter(ctrl)
		processFinder = process.NewMockVmiProcessFinder(ctrl)

		memLockCallVmi = nil
		memLockCallFinder = nil
		memlockConfigReturnError = nil

		configureMemlockRLimitsFunc = mockMemlockConfigurator

		ctx = context.TODO()
		testVmi = libvmi.New()
	})

	It("must return an error if a request for another node is received ", func() {
		callback := NewMemlockNodeHookCallback(
			callbackTestNodeName,
			"nonHandledHookPoint",
			vmiFilter,
			processFinder,
		)

		req := &plugin.NodeHookRequest{
			HookPoint: plugin.PreVMStart,
			VMI:       testVmi,
			NodeName:  "another-node-we-dont-expect",
		}

		res := callback.ExecuteNodeHook(ctx, req)
		Expect(res).NotTo(BeNil())
		Expect(res).To(Equal(
			fmt.Errorf("expected to handle requests on node %q. Got \"another-node-we-dont-expect\"",
				callbackTestNodeName,
			)))
	})

	It("must return an error if an unexpected hook point is handled", func() {
		callback := NewMemlockNodeHookCallback(
			callbackTestNodeName,
			"nonHandledHookPoint",
			vmiFilter,
			processFinder,
		)

		req := &plugin.NodeHookRequest{
			HookPoint: plugin.PreVMStart,
			VMI:       testVmi,
			NodeName:  callbackTestNodeName,
		}

		res := callback.ExecuteNodeHook(ctx, req)
		Expect(res).NotTo(BeNil())
		Expect(res).To(Equal(fmt.Errorf("expected to handle hook point \"nonHandledHookPoint\". Got \"PreVMStart\"")))
	})

	It("must not take any action if the vmi does not match the filter", func() {
		callback := NewMemlockNodeHookCallback(
			callbackTestNodeName,
			plugin.PreVMStart,
			vmiFilter,
			processFinder,
		)

		vmiFilter.EXPECT().Matches(testVmi).Return(false)

		req := &plugin.NodeHookRequest{
			HookPoint: plugin.PreVMStart,
			VMI:       testVmi,
			NodeName:  callbackTestNodeName,
		}

		res := callback.ExecuteNodeHook(ctx, req)
		Expect(res).To(BeNil())
		Expect(memLockCallVmi).To(BeNil())
		Expect(memLockCallFinder).To(BeNil())
	})

	It("must configure the memlock limits of the vmi process", func() {
		callback := NewMemlockNodeHookCallback(
			callbackTestNodeName,
			plugin.PreVMStart,
			vmiFilter,
			processFinder,
		)

		vmiFilter.EXPECT().Matches(testVmi).Return(true)

		req := &plugin.NodeHookRequest{
			HookPoint: plugin.PreVMStart,
			VMI:       testVmi,
			NodeName:  callbackTestNodeName,
		}

		res := callback.ExecuteNodeHook(ctx, req)
		Expect(res).To(BeNil())
		Expect(memLockCallVmi).To(Equal(testVmi))
		Expect(memLockCallFinder).To(Equal(processFinder))
	})

	It("must return an error if an error occurred configuring the memlock limits of the vmi process", func() {
		callback := NewMemlockNodeHookCallback(
			callbackTestNodeName,
			plugin.PreVMStart,
			vmiFilter,
			processFinder,
		)

		vmiFilter.EXPECT().Matches(testVmi).Return(true)
		memlockConfigReturnError = fmt.Errorf("error setting memlock rlimits on a process")

		req := &plugin.NodeHookRequest{
			HookPoint: plugin.PreVMStart,
			VMI:       testVmi,
			NodeName:  callbackTestNodeName,
		}

		res := callback.ExecuteNodeHook(ctx, req)
		Expect(res).To(Equal(memlockConfigReturnError))
		Expect(memLockCallVmi).To(Equal(testVmi))
		Expect(memLockCallFinder).To(Equal(processFinder))
	})

})
