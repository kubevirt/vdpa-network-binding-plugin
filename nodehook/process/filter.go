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

//go:generate mockgen -source $GOFILE -package=$GOPACKAGE -destination=generated_mock_$GOFILE

import (
	"fmt"
)

const VirtQemudProcessExecutableName = "virtqemud"

type ProcessFilter interface {
	// Returns true if a ProcessInfo meets required conditions
	Matches(p *ProcessInfo) bool

	// Returns some information about the Filter which will be used
	// during error reporting
	Info() string
}

type ExecutableNameProcessFilter struct {
	execName string
}

func (f *ExecutableNameProcessFilter) Info() string {
	return fmt.Sprintf("%s process", f.execName)
}

func (f *ExecutableNameProcessFilter) Matches(p *ProcessInfo) bool {
	return p.ExecName == f.execName
}

func NewVirtqemudProcessFilter() *ExecutableNameProcessFilter {
	return &ExecutableNameProcessFilter{VirtQemudProcessExecutableName}
}
