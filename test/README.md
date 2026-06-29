# vdpa-network-binding-plugin tests

This directory contains the vdpa-network-binding-plugin integration
tests. These are supposed to make sure that the network binding plugin
is supported by KubeVirt and that it carries out proper VMI
configuration under different circumstances.

## Test environment description and limitations

Tests might be run on an environment with hardware backing up the vdpa
devices. In that case, how KubeVirt and the other components involved
integrate is also tested. For that, it's recommended to rely on the
[sriov-network-operator][sriov-op], the
[sriov-network-device-plugin][sriov-dp] and [ovs-cni][ovs-cni]. However,
we won't cover that setup in this document.

[sriov-op]: https://github.com/k8snetworkplumbingwg/sriov-network-operator
[sriov-dp]: https://github.com/k8snetworkplumbingwg/sriov-network-device-plugin
[ovs-cni]: github.com/k8snetworkplumbingwg/ovs-cni

In case that specific hardware is not available, tests can be also run
in a k8s cluster by using `vdpasim_net` devices. These don't allow
flowing traffic between VMs and are not supposed to be used in any
production environment by any means. Device plugin and CNI
implementations that manage `vdpasim_net` those are not available out
there. However, this tests directory offers dummy DP/CNI implementations
for this case that enable vdpa-network-binding-plugin testing without
special hardware requirements. See
[vdpa-sim-net-cni](./vdpa-sim-net-cni) and
[vdpa-sim-net-device-plugin](./vdpa-sim-net-device-plugin).

### vdpa-sim-net-device-plugin

The device plugin loads the kernel modules needed to create the
`vdpasim_net` mgmtdev, vdpa devices and bind them to the `vhost_vdpa`
driver.

Then, it creates the `vdpasim_net` device, creates some vdpa devices
backed by it, advertises them to the kubelet and creates and fills the
appropriate device-info files. The vdpa devices can be split into
multiple resource groups. That way it can be ensured that the
configuration a device on a group matches no matter the assigned node.

### vdpa-sim-net-cni

The CNI does not do much. It receives a `deviceID` and the path to the
device-info file, fetches the assigned vdpa device configuration
information and fills the network-status fields that are relevant.

Note that it is important to enable the `CNIDeviceInfoFile` capability
in the the `vdpa-sim-net-cni` network attachment definition.

### Dependency deployment

In case testing needs to be performed in an environment without special
hardware, vdpa-sim-net device plugin and CNI need to be present in that
cluster. For that, apply the [test dependency manifests](./manifests),
or run `make sync_test_dependencies`.

Manifests will create a privileged namespace in which the test
dependencies (device plugin, CNI and network attachment definition) will
be kept. They will also create the device plugin and CNI Daemonsets as
well as a network-attachment-definition ready to work with the default
configuration.

### Important notes

- kind clusters are not supported by the vdpa-sim-net-cni and
  device-plugin unless special care is taken in the cluster
  configuration. However, we won't cover that here. For that reason, we
  recommend using bare-metal or emulated k8s deployments to run these
  tests. k8s [KubeVirtCI][kubevirtci] providers `k8s-1.35` and above are
  supported. Looking at the [running tests][#running-tests] section may
  help.
- Multus is required.
- Kubevirt network resources injector is also required.

For that reason, we recommend setting up a KubeVirtCI provided cluster
with the following environment variables:
```
export KUBEVIRT_PROVIDER=k8s-1.35
export KUBEVIRT_NUM_NODES=3
export KUBEVIRT_WITH_MULTUS=true
export KUBEVIRT_DEPLOY_NETWORK_RESOURCES_INJECTOR=true
```

The number of nodes is a personal choice, however, a setup with less
than 3 nodes won't support testing live-migration.

[kubevirtci]: https://github.com/kubevirt/kubevirtci

## Test environment setup

### Setting up the cluster

A cluster needs to be up and running to run the integration tests. This
repository contains kubevirtci as a git submodule so it is easier to set
it up. Just go to the outtermost directory and run:

```bash
make kubevirtci_init
make cluster_up
```

- `kubevirtci_init` will make sure that the kubevirtci submodule is
  initialized in your local fork.
- `cluster_up` will set the cluster up.

By default, it will configure the following environment variables:
```
export KUBEVIRT_PROVIDER=k8s-1.35
export KUBEVIRT_NUM_NODES=2
export KUBEVIRT_WITH_MULTUS=true
export KUBEVIRT_DEPLOY_NETWORK_RESOURCES_INJECTOR=true
```

`KUBEVIRT_PROVIDER` and `KUBEVIRT_NUM_NODES` can be configured too.

The kubevirtci version is automatically bumped periodically. Running
`kubevirtci_init` from time to time will make sure that the same cluster
version as the one in the CI is being set in the local test environment.

### Installing KubeVirt

Once the cluster is up and running, KubeVirt needs to be installed.
There is an existing `make` directive that comes in handy:
```bash
make cluster_sync_kubevirt
```

When running that, the `KUBEVIRT_SYNC_VERSION` environment variable will
control which KubeVirt version will be deployed to the cluster. It
admits three types of values:
- 'latest': The default behavior. It will install the latest available
   official release of KubeVirt.
- 'nightly': It will install the latest available nightly build of
  KubeVirt.
- Anything matching KubeVirt's semver versioning schema such as:
  `v1.7.1`, `v1.8.0-alpha.0`...

Note that to run this command either `KUBEVIRTCI_TAG` or
`KUBEVIRTCI_GOCLI_CONTAINER` may need to be set.

### Installing test dependencies

Make sure that the dependency images were built and pushed into a
registry that is reachable by the test cluster, i.e. set appropriate
`IMAGE_REGISTRY` and `PUSH_REGISTRY`. Note that if tests are meant to be
run on a kubevirtci cluster, images don't need to be pushed to quay.io
or other external registries. However, the registry is reached from
differently in localhost compared to the kubevirtci cluster. Then, set
`IMAGE_REGISTRY` to match how kubelet will succeed pulling images, and
`PUSH_REGISTRY` so images are pushed correctly to the local kubevirtci
registry. Ignoring TLS verification might be also needed in such case.
In simpler terms:
```bash
export IMAGE_REGISTRY="registry:5000"
export PUSH_REGISTRY="localhost:$(./test/cluster/cli.sh ports registry | tr -d '\r')"
export REQUIRE_IMAGE_PUSH_TLS_VERIFICATION=false
```

Note that if an external cluster or registry are being used, just
setting an appropriate `IMAGE_REGISTRY` is enough.

Then,
```bash
make image_test_dependencies
make push_test_dependencies
make sync_test_dependencies
```
That will deploy the vdpa-sim-net-{cni,device-plugin} components to the
test cluster.

### Installing the network binding plugin

With similar environment variables as before,
```bash
make images
make push
make sync
```

## Running integration tests
