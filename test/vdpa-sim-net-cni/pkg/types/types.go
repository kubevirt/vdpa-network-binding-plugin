package types

import (
	"github.com/containernetworking/cni/pkg/types"
)

type RuntimeConfig struct {
	types.CommonArgs
	CNIDeviceInfoFile string `json:"CNIDeviceInfoFile,omitempty"`
}

type NetConf struct {
	types.NetConf
	DeviceID      string         `json:"deviceID"`
	RuntimeConfig *RuntimeConfig `json:"runtimeConfig,omitempty"`
}
