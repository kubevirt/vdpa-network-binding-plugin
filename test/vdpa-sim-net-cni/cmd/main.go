package main

import (
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/utils/buildversion"

	"kubevirt.io/vdpa-network-binding-plugin/test/vdpa-sim-net-cni/pkg/plugin"
)

func main() {
	skel.PluginMainFuncs(skel.CNIFuncs{
		Add:   plugin.CmdAdd,
		Check: plugin.CmdCheck,
		Del:   plugin.CmdDel,
	}, version.All, buildversion.BuildString("vdpasimnet"))
}
