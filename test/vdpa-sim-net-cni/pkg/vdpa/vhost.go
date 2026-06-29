package vdpa

import (
	"fmt"

	"github.com/vishvananda/netlink"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	"github.com/containernetworking/plugins/pkg/ns"

	netv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	current "github.com/containernetworking/cni/pkg/types/100"
	"kubevirt.io/vdpa-network-binding-plugin/test/vdpa-sim-net-cni/pkg/types"
)

func configureVhostVdpa(deviceInfo *netv1.DeviceInfo, netconf *types.NetConf, args *skel.CmdArgs) error {
	hostIface := &current.Interface{}
	contIface := &current.Interface{}

	contIface.Name = args.IfName
	devName := netconf.DeviceID

	vdpaConfig, err := netlink.VDPAGetDevConfigByName(devName)
	if err != nil {
		return fmt.Errorf("failed to fetch config for vdpa device %q: %w", devName, err)
	}
	contIface.Mac = vdpaConfig.Net.Cfg.MACAddr.String()
	contIface.Mtu = int(vdpaConfig.Net.Cfg.MTU)

	contNetns, err := ns.GetNS(args.Netns)
	if err != nil {
		return fmt.Errorf("failed to get container netns %q: %v", args.Netns, err)
	}
	contIface.Sandbox = contNetns.Path()

	result := &current.Result{
		Interfaces: []*current.Interface{hostIface, contIface},
	}
	return cnitypes.PrintResult(result, netconf.CNIVersion)
}
