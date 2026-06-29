package plugin

import (
	"encoding/json"
	"fmt"

	"github.com/containernetworking/cni/pkg/skel"

	"kubevirt.io/vdpa-network-binding-plugin/test/vdpa-sim-net-cni/pkg/deviceinfo"
	"kubevirt.io/vdpa-network-binding-plugin/test/vdpa-sim-net-cni/pkg/types"
	"kubevirt.io/vdpa-network-binding-plugin/test/vdpa-sim-net-cni/pkg/vdpa"
)

func loadNetConf(bytes []byte) (*types.NetConf, error) {
	netconf := &types.NetConf{}
	if err := json.Unmarshal(bytes, netconf); err != nil {
		return nil, fmt.Errorf("failed to load netconf: %v", err)
	}
	return netconf, nil
}

func CmdAdd(args *skel.CmdArgs) error {
	netconf, err := loadNetConf(args.StdinData)
	if err != nil {
		return err
	}

	devInfo, err := deviceinfo.GetDeviceInfo(netconf)
	if err != nil {
		return err
	}

	return vdpa.Configure(devInfo, netconf, args)
}

func CmdDel(args *skel.CmdArgs) error {
	return nil
}

func CmdCheck(args *skel.CmdArgs) error {
	return nil
}
