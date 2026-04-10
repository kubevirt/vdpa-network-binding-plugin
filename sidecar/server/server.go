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

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"

	vmschema "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/pkg/network/downwardapi"

	"kubevirt.io/kubevirt/pkg/hooks"
	hooksInfo "kubevirt.io/kubevirt/pkg/hooks/info"
	hooksV1alpha2 "kubevirt.io/kubevirt/pkg/hooks/v1alpha2"

	"kubevirt.io/vdpa-network-binding-plugin/sidecar/callback"
	"kubevirt.io/vdpa-network-binding-plugin/sidecar/domain"
	"kubevirt.io/vdpa-network-binding-plugin/sidecar/symlink"
)

type InfoServer struct {
	Version string
}

func (s InfoServer) Info(_ context.Context, _ *hooksInfo.InfoParams) (*hooksInfo.InfoResult, error) {
	return &hooksInfo.InfoResult{
		Name: "network-vdpa-binding",
		Versions: []string{
			s.Version,
		},
		HookPoints: []*hooksInfo.HookPoint{
			{
				Name:     hooksInfo.OnDefineDomainHookPointName,
				Priority: 0,
			},
		},
	}, nil
}

type V1alpha2Server struct{}

func (s V1alpha2Server) OnDefineDomain(_ context.Context, params *hooksV1alpha2.OnDefineDomainParams) (*hooksV1alpha2.OnDefineDomainResult, error) {
	vmi := &vmschema.VirtualMachineInstance{}
	if err := json.Unmarshal(params.GetVmi(), vmi); err != nil {
		return nil, fmt.Errorf("failed to unmarshal VMI: %v", err)
	}

	netInfoPath := path.Join(downwardapi.MountPath, downwardapi.NetworkInfoVolumePath)
	netInfo, err := domain.GetDownwardAPINetworkInfo(netInfoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read network-info: %v", err)
	}

	symlinkFactory := symlink.NewSharedSymlinkFactory()

	containerName := os.Getenv(hooks.ContainerNameEnvVar)
	if containerName == "" {
		return nil, fmt.Errorf("failed to get %s environment variable", hooks.ContainerNameEnvVar)
	}

	vdpaConfigurator, err := domain.NewVdpaNetworkConfigurator(vmi.Spec.Domain.Devices.Interfaces, vmi.Spec.Networks, netInfo, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to create vdpa configurator: %v", err)
	}

	for vdpaPath, symlinkName := range vdpaConfigurator.VdpaPathsToSymlinkNames() {
		err = symlinkFactory.CreateSharedSymlink(vdpaPath, symlinkName)
		if err != nil {
			return nil, err
		}
	}

	newDomainXML, err := callback.OnDefineDomain(params.GetDomainXML(), vdpaConfigurator)
	if err != nil {
		return nil, err
	}

	return &hooksV1alpha2.OnDefineDomainResult{
		DomainXML: newDomainXML,
	}, nil
}

func (s V1alpha2Server) PreCloudInitIso(_ context.Context, params *hooksV1alpha2.PreCloudInitIsoParams) (*hooksV1alpha2.PreCloudInitIsoResult, error) {
	return &hooksV1alpha2.PreCloudInitIsoResult{
		CloudInitData: params.GetCloudInitData(),
	}, nil
}
