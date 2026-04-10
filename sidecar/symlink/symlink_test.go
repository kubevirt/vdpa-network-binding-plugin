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
 * Copyright 2023 Red Hat, Inc.
 *
 */

package symlink_test

import (
	"os"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"kubevirt.io/vdpa-network-binding-plugin/sidecar/symlink"
)

const DEFAULT_TARGET = "/dev/vhost-vdpa-target-path"

var _ = Describe("vdpa symlink factory", func() {
	Context("sidecar-side symlink creation", func() {
		var tmpDir string
		var factory *symlink.SharedSymlinkFactory
		BeforeEach(func() {
			tmpDir = GinkgoT().TempDir()
			factory = &symlink.SharedSymlinkFactory{Dir: tmpDir}
		})

		It("should create the symlink", func() {
			name := "newSymlink"
			err := factory.CreateSharedSymlink(DEFAULT_TARGET, name)
			Expect(err).ToNot(HaveOccurred())

			symlinkPath := path.Join(tmpDir, name)
			symlink, err := os.Readlink(symlinkPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(symlink).To(Equal(DEFAULT_TARGET))
		})

		It("should be safe to run it twice", func() {
			name := "symlinkTwice"
			err := factory.CreateSharedSymlink(DEFAULT_TARGET, name)
			Expect(err).ToNot(HaveOccurred())

			symlinkPath := path.Join(tmpDir, name)
			symlink, err := os.Readlink(symlinkPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(symlink).To(Equal(DEFAULT_TARGET))

			err = factory.CreateSharedSymlink(DEFAULT_TARGET, name)
			Expect(err).ToNot(HaveOccurred())
			symlink, err = os.Readlink(symlinkPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(symlink).To(Equal(DEFAULT_TARGET))
		})

		It("should return error if symlink points into another path", func() {
			name := "symlinkPointsSomewhereElse"
			symlinkPath := path.Join(tmpDir, name)

			err := os.Symlink("/var/lib/anothertarget", symlinkPath)
			Expect(err).ToNot(HaveOccurred())
			err = factory.CreateSharedSymlink(DEFAULT_TARGET, name)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("instantiation", func() {
		It("should point into the sidecar hooks directory", func() {
			factory := symlink.NewSharedSymlinkFactory()
			Expect(factory.Dir).To(Equal("/var/run/kubevirt-hooks"))
		})
	})
})
