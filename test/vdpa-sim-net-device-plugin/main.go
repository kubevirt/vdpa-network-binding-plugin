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
 */

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vishvananda/netlink"

	nadv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	nadutils "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/utils"
)

const (
	resourceNamePrefix = "devices.kubevirt.io/"
	kubeletSocket      = "/var/lib/kubelet/device-plugins/kubelet.sock"
	devicePluginPath   = "/var/lib/kubelet/device-plugins"
	deviceInfoBasePath = "/var/run/k8s.cni.cncf.io/devinfo/dp"
	simnetMgmtdev      = "vdpasim_net"
	vdpaSysBusPath     = "/sys/bus/vdpa/devices"
)

var modules = []string{"vdpa", "vhost_vdpa", "vdpa_sim", "vdpa_sim_net"}

func main() {
	configPath := flag.String("config", "", "Path to the device plugin configuration file")
	flag.Parse()

	if *configPath == "" {
		log.Fatalf("Device plugin configuration path not provided")
	}

	dpConfig, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load device plugin config: %q", err)
	}

	if err := loadModules(); err != nil {
		log.Fatalf("Failed to load kernel modules: %v", err)
	}

	if err := createVDPADevices(dpConfig); err != nil {
		log.Fatalf("Failed to create vDPA device: %v", err)
	}

	var plugins []*VDPADevicePlugin
	for _, resource := range dpConfig.Resources {
		resourceName := resourceNamePrefix + resource.Name
		devices := discoverAllocatableDevices(resource)

		if err := createDeviceInfoFiles(resourceName, devices); err != nil {
			log.Fatalf("Failed to create device info files for %s: %v", resourceName, err)
		}

		plugins = append(plugins, NewVDPADevicePlugin(resourceName, devices))
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

	done := make(chan struct{})
	var wg sync.WaitGroup
	for _, p := range plugins {
		wg.Add(1)
		go func(plugin *VDPADevicePlugin) {
			defer wg.Done()
			for {
				if err := plugin.Serve(); err != nil {
					log.Fatalf("Failed to start gRPC server for %s: %v", plugin.resourceName, err)
				}
				if err := plugin.Register(); err != nil {
					log.Fatalf("Failed to register %s with kubelet: %v", plugin.resourceName, err)
				}
				log.Printf("Device plugin registered, serving resource %s", plugin.resourceName)

				select {
				case <-done:
					plugin.Stop()
					return
				case <-plugin.WatchSocket():
					log.Printf("Re-registering %s after kubelet restart", plugin.resourceName)
					plugin.Stop()
				}
			}
		}(p)
	}

	<-sig
	log.Println("Shutting down")
	close(done)
	wg.Wait()
	bestEffortDeviceInfoCleanup(dpConfig)
	bestEffortVDPADeviceCleanup(dpConfig)
}

func loadModules() error {
	for _, mod := range modules {
		if out, err := exec.Command("modprobe", mod).CombinedOutput(); err != nil {
			return fmt.Errorf("modprobe %s: %s: %w", mod, string(out), err)
		}
		log.Printf("Loaded module %s", mod)
	}
	return nil
}

func createVDPADevices(dpConfig *VdpaSimNetDevicePluginConfiguration) error {
	for _, resource := range dpConfig.Resources {
		for _, config := range resource.Configs {
			sysPath := filepath.Join(vdpaSysBusPath, config.Name)
			if _, err := os.Stat(sysPath); err == nil {
				log.Printf("vDPA device %s already exists, skipping creation", config.Name)
				continue
			}

			devParams := netlink.VDPANewDevParams{}
			if config.Mac != nil {
				mac, err := net.ParseMAC(*config.Mac)
				if err != nil {
					return fmt.Errorf("invalid mac address %q for device %s", *config.Mac, config.Name)
				}

				devParams.MACAddr = mac
			}

			if config.MTU != nil {
				devParams.MTU = uint16(*config.MTU)
			}

			if err := netlink.VDPANewDev(config.Name, "", simnetMgmtdev, devParams); err != nil {
				return fmt.Errorf("vdpa dev add: %w", err)
			}
			log.Printf("Created vDPA device %s", config.Name)
		}
	}
	return nil
}

func discoverAllocatableDevices(resource *VdpaSimNetResources) []*AllocatableVdpaDevice {
	var devices []*AllocatableVdpaDevice
	for _, config := range resource.Configs {
		device, err := discoverAllocatableDevice(config.Name)
		if err != nil {
			log.Printf("warning: couldn't find vhost path of device %q", config.Name)
			continue
		}

		devices = append(devices, device)
	}
	return devices
}

func createDeviceInfoFiles(resourceName string, devices []*AllocatableVdpaDevice) error {
	for _, device := range devices {
		if err := createDeviceInfo(resourceName, device); err != nil {
			return err
		}
	}

	return nil
}

func discoverAllocatableDevice(vdpaDeviceName string) (*AllocatableVdpaDevice, error) {
	sysDir := filepath.Join(vdpaSysBusPath, vdpaDeviceName)
	for i := 0; i < 10; i++ {
		entries, err := os.ReadDir(sysDir)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", sysDir, err)
		}
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "vhost-vdpa-") {
				devPath := filepath.Join("/dev", e.Name())
				if _, err := os.Stat(devPath); err == nil {
					return &AllocatableVdpaDevice{
						Name: vdpaDeviceName,
						Path: devPath,
					}, nil
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("no vhost-vdpa device found for %s after 5s", vdpaDeviceName)
}

func createDeviceInfo(resourceName string, device *AllocatableVdpaDevice) error {
	info := nadv1.DeviceInfo{
		Type:    nadv1.DeviceInfoTypeVDPA,
		Version: nadv1.DeviceInfoVersion,
		Vdpa: &nadv1.VdpaDevice{
			Path:         device.Path,
			ParentDevice: simnetMgmtdev,
			Driver:       "vhost",
		},
	}

	return nadutils.SaveDeviceInfoForDP(resourceName, device.Name, &info)
}

func deleteDeviceInfo(resourceName, vdpaDeviceName string) {
	err := nadutils.CleanDeviceInfoForDP(resourceName, vdpaDeviceName)
	if err != nil {
		log.Printf("error: couldn't clean device plugin device info for %q:%q: %v", resourceName, vdpaDeviceName, err)
	}
}

func bestEffortDeviceInfoCleanup(dpConfig *VdpaSimNetDevicePluginConfiguration) {
	for _, resource := range dpConfig.Resources {
		resourceName := resourceNamePrefix + resource.Name
		for _, devConfig := range resource.Configs {
			deleteDeviceInfo(resourceName, devConfig.Name)
		}
	}
}

func bestEffortVDPADeviceCleanup(dpConfig *VdpaSimNetDevicePluginConfiguration) {
	for _, resource := range dpConfig.Resources {
		for _, config := range resource.Configs {
			err := netlink.VDPADelDev(config.Name)
			if err != nil {
				log.Printf("Warning: failed to delete vDPA device: %s: %v", config.Name, err)
			}
		}
	}
}

func loadConfig(path string) (*VdpaSimNetDevicePluginConfiguration, error) {
	var dpConfig VdpaSimNetDevicePluginConfiguration

	f, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(f, &dpConfig)
	if err != nil {
		return nil, err
	}

	return &dpConfig, nil
}
