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

package main

import (
	"log"
	"os"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/kubevirt/plugins/pkg/sdk/plugin"
	"kubevirt.io/kubevirt/pkg/virt-handler/isolation"

	"kubevirt.io/vdpa-network-binding-plugin/nodehook/callback"
	"kubevirt.io/vdpa-network-binding-plugin/nodehook/process"
	"kubevirt.io/vdpa-network-binding-plugin/nodehook/vmi"
)

const (
	defaultVdpaPluginName string = "vdpa"
	NodeNameEnvVar        string = "NODE_NAME"
)

func main() {
	networkBindingPluginName := flag.String(
		"plugin-name",
		defaultVdpaPluginName,
		"Network binding plugin name registered in KubeVirt",
	)
	flag.Parse()

	nodeName, ok := os.LookupEnv(NodeNameEnvVar)
	if !ok {
		log.Printf("ERROR: node name can't be elucidated")
		os.Exit(1)
	}

	log.Printf("INFO: starting nodehook on node %s, tracking binding plugin %s", nodeName, *networkBindingPluginName)
	vmiFilter := vmi.NewNBPVMIFilter(*networkBindingPluginName)

	launcherDetector := isolation.NewSocketBasedIsolationDetector()
	virtqemudProcessFilter := process.NewVirtqemudProcessFinder(launcherDetector)

	preStartCallback := callback.NewMemlockNodeHookCallback(
		nodeName,
		string(plugin.PreVMStart),
		vmiFilter,
		virtqemudProcessFilter,
	)
	preMigrationTargetCallback := callback.NewMemlockNodeHookCallback(
		nodeName,
		string(plugin.PreMigrationTarget),
		vmiFilter,
		virtqemudProcessFilter,
	)

	preStartVMHandler := plugin.NodeHandler(preStartCallback).
		WithFailureStrategy(plugin.Fail).
		WithTimeout(10 * time.Second)
	preMigrationTargetHandler := plugin.NodeHandler(preMigrationTargetCallback).
		WithFailureStrategy(plugin.Fail).
		WithTimeout(10 * time.Second)

	p := plugin.New("vdpa-network-binding-plugin-node-plugin-hook").
		WithNodeHook(plugin.PreVMStart, preStartVMHandler).
		WithNodeHook(plugin.PreMigrationTarget, preMigrationTargetHandler)
	log.Printf("INFO: serving node hook")
	p.Execute()
}
