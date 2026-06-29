package vdpa

import (
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"

	netv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	"kubevirt.io/vdpa-network-binding-plugin/test/vdpa-sim-net-cni/pkg/types"
)

type VdpaDeviceType string

const (
	VdpaDeviceTypeVhost   = "vhost"
	VdpaDeviceTypeUnknown = ""
)

func IsVdpa(deviceInfo *netv1.DeviceInfo) bool {
	return deviceInfo != nil && deviceInfo.Type == "vdpa" && deviceInfo.Vdpa != nil
}

func GetVdpaDeviceType(deviceInfo *netv1.DeviceInfo) (VdpaDeviceType, error) {
	if !IsVdpa(deviceInfo) {
		return "", fmt.Errorf("DeviceInfo %+v does not represent a vdpa device", deviceInfo)
	}

	switch deviceInfo.Vdpa.Driver {
	case "vhost":
		return VdpaDeviceTypeVhost, nil
	default:
		return VdpaDeviceTypeUnknown,
			fmt.Errorf("unknown vdpa device type: %q", deviceInfo.Vdpa.Driver)
	}
}

func Configure(deviceInfo *netv1.DeviceInfo, netconf *types.NetConf, args *skel.CmdArgs) error {
	vdpaType, err := GetVdpaDeviceType(deviceInfo)
	if err != nil {
		return err
	}

	switch vdpaType {
	case VdpaDeviceTypeVhost:
		return configureVhostVdpa(deviceInfo, netconf, args)
	}

	return fmt.Errorf("unknown vdpa device type: %q", vdpaType)
}
