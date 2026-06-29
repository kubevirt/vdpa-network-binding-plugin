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
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type VDPADevicePlugin struct {
	pluginapi.UnimplementedDevicePluginServer
	resourceName string
	devices      []*AllocatableVdpaDevice
	socketPath   string
	server       *grpc.Server
	stop         chan struct{}
	stopped      chan struct{}
}

func NewVDPADevicePlugin(resourceName string, devices []*AllocatableVdpaDevice) *VDPADevicePlugin {
	socketName := strings.ReplaceAll(resourceName, "/", "-") + ".sock"
	return &VDPADevicePlugin{
		resourceName: resourceName,
		devices:      devices,
		socketPath:   filepath.Join(devicePluginPath, socketName),
		stop:         make(chan struct{}),
		stopped:      make(chan struct{}),
	}
}

func (p *VDPADevicePlugin) Serve() error {
	_ = os.Remove(p.socketPath)

	p.stop = make(chan struct{})
	p.stopped = make(chan struct{})

	listener, err := net.Listen("unix", p.socketPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", p.socketPath, err)
	}

	p.server = grpc.NewServer()
	pluginapi.RegisterDevicePluginServer(p.server, p)

	go func() {
		defer close(p.stopped)
		if err := p.server.Serve(listener); err != nil {
			log.Printf("gRPC server exited: %v", err)
		}
	}()

	return nil
}

// WatchSocket polls for the plugin socket being deleted, which signals
// that kubelet has restarted and the plugin needs to re-register.
func (p *VDPADevicePlugin) WatchSocket() <-chan struct{} {
	deleted := make(chan struct{})
	go func() {
		defer close(deleted)
		for {
			select {
			case <-p.stop:
				return
			case <-time.After(5 * time.Second):
				if _, err := os.Stat(p.socketPath); os.IsNotExist(err) {
					log.Printf("Socket %s deleted, kubelet likely restarted", p.socketPath)
					return
				}
			}
		}
	}()
	return deleted
}

func (p *VDPADevicePlugin) Register() error {
	conn, err := grpc.NewClient(
		"unix://"+kubeletSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connecting to kubelet: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Couldn't close device plugin registration client")
		}
	}()

	_, err = pluginapi.NewRegistrationClient(conn).Register(
		context.Background(),
		&pluginapi.RegisterRequest{
			Version:      pluginapi.Version,
			Endpoint:     filepath.Base(p.socketPath),
			ResourceName: p.resourceName,
		},
	)
	if err != nil {
		return fmt.Errorf("registering with kubelet: %w", err)
	}
	return nil
}

func (p *VDPADevicePlugin) Stop() {
	close(p.stop)
	if p.server != nil {
		p.server.Stop()
	}
	<-p.stopped
	if err := os.Remove(p.socketPath); err != nil {
		log.Printf("Couldn't remove socket: %s", p.socketPath)
	}
}

func (p *VDPADevicePlugin) GetDevicePluginOptions(_ context.Context, _ *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func (p *VDPADevicePlugin) ListAndWatch(_ *pluginapi.Empty, stream pluginapi.DevicePlugin_ListAndWatchServer) error {
	var devices []*pluginapi.Device

	for _, device := range p.devices {
		devices = append(devices, &pluginapi.Device{
			ID:     device.Name,
			Health: pluginapi.Healthy,
		})
	}

	if err := stream.Send(&pluginapi.ListAndWatchResponse{
		Devices: devices,
	}); err != nil {
		return err
	}

	select {
	case <-p.stop:
		return nil
	case <-stream.Context().Done():
		return nil
	}
}

func (p *VDPADevicePlugin) findDevice(deviceID string) (*AllocatableVdpaDevice, error) {
	for _, device := range p.devices {
		if device.Name == deviceID {
			return device, nil
		}
	}

	return nil, fmt.Errorf("deviceID %q not found", deviceID)
}

func (p *VDPADevicePlugin) Allocate(_ context.Context, req *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	var responses []*pluginapi.ContainerAllocateResponse
	for _, request := range req.ContainerRequests {
		var devices []*pluginapi.DeviceSpec

		for _, deviceID := range request.DevicesIds {
			device, err := p.findDevice(deviceID)
			if err != nil {
				return nil, err
			}

			devices = append(devices, &pluginapi.DeviceSpec{
				HostPath:      device.Path,
				ContainerPath: device.Path,
				Permissions:   "rw",
			})
		}

		responses = append(responses, &pluginapi.ContainerAllocateResponse{
			Devices: devices,
		})
	}

	return &pluginapi.AllocateResponse{ContainerResponses: responses}, nil
}

func (p *VDPADevicePlugin) GetPreferredAllocation(_ context.Context, _ *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}

func (p *VDPADevicePlugin) PreStartContainer(_ context.Context, _ *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}
