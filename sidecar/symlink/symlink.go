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

package symlink

import (
	"fmt"
	"os"
	"path"

	"kubevirt.io/kubevirt/pkg/hooks"
)

func SharedComputeSymlinkPath(containerName, symlinkName string) string {
	return path.Join(hooks.HookSocketsSharedDirectory, containerName, symlinkName)
}

type SharedSymlinkFactory struct {
	Dir string
}

func NewSharedSymlinkFactory() *SharedSymlinkFactory {
	return &SharedSymlinkFactory{
		Dir: hooks.HookSocketsSharedDirectory,
	}
}

// Creates a symlink in the directory shared between the hook sidecar
// to oldPath, given the desired symlink name.
func (f *SharedSymlinkFactory) CreateSharedSymlink(oldPath, symlinkName string) error {
	symlinkPath := path.Join(f.Dir, symlinkName)

	err := os.Symlink(oldPath, symlinkPath)
	if err == nil {
		return nil
	} else if !os.IsExist(err) {
		return err
	}

	sl, err := os.Readlink(symlinkPath)
	if err != nil {
		return err
	}

	if sl != oldPath {
		return fmt.Errorf(
			"symlink %s exists and points into %s but expected %s",
			symlinkPath, sl, oldPath,
		)
	}
	return nil
}
