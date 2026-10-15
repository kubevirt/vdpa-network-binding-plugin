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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("executable process filter", func() {
	DescribeTable("filters out processes with different executable names",
		func(execName string, expectedMatch bool) {
			filter := ExecutableNameProcessFilter{"wantedExecutable"}
			info := &ProcessInfo{ExecName: execName}
			Expect(filter.Matches(info)).To(Equal(expectedMatch))
		},
		Entry("process with different executable", "other", false),
		Entry("process with matching executable", "wantedExecutable", true),
	)

	It("return appropriate filter information", func() {
		execName := "wantedExecutable"
		filter := ExecutableNameProcessFilter{execName}
		Expect(filter.Info()).To(Equal("wantedExecutable process"))
	})
})

var _ = Describe("virtqemud process filter", func() {
	DescribeTable("filters out processes that are not named virtqemud",
		func(execName string, expectedMatch bool) {
			filter := NewVirtqemudProcessFilter()
			info := &ProcessInfo{ExecName: execName}
			Expect(filter.Matches(info)).To(Equal(expectedMatch))
		},
		Entry("process with different executable", "other", false),
		Entry("process running virtqemud", "virtqemud", true),
	)

	It("return appropriate filter information", func() {
		filter := NewVirtqemudProcessFilter()
		Expect(filter.Info()).To(Equal("virtqemud process"))
	})
})
