# vdpa-network-binding-plugin

This repository contains the source code that brings secondary [vDPA][vdpa]
network interfaces to KubeVirt.

The repository contains two components: a sidecar, and a KubeVirt node
plugin hook. The sidecar is deployed as a sidecar container in the
virt-launcher pods of VMs that require a vDPA network interface. It is
in charge of mutating the domainXML to include the vDPA interface
configuration. The node hook increases the memlock RLimits of the
virtqemud processes running in the virt-launchers that hold VMs with
VDPA interfaces. It is called before VMs are started or received in the
migration target launcher. It increases the memlock RLimits of VMs with
VDPA network interfaces.

[vdpa]: https://vdpa-dev.gitlab.io/


## Build
To build the components, export the `IMAGE_REGISTRY` and `IMAGE_TAG`
environment variables according to your needs and run `make images`.

You can manually push the built images to the registry, or just run
`make push`.


## Deploy
### Sidecar
After having exported the right `IMAGE_REGISTRY` and `IMAGE_TAG`
environment variables, run:
```
$ make manifests
$ kubectl patch -n kubevirt kubevirts kubevirt --type merge \
  --patch-file manifests/vdpa-sidecar-patch.yaml
```

### Node plugin hook
As with the other components, setting appopriate `IMAGE_REGISTRY`,
`IMAGE_TAG` and `PUSH_REGISTRY` help tweaking manifests:
```
$ make manifests
```

**NOTE**: This component currently requires the `Plugins` feature gate
of KubeVirt to be enabled.

Then,
```
$ kubectl apply -f manifests/vdpa-node-plugin-hook.yaml
```

deploys the node hook into all cluster's nodes by a daemonset and
registers the KubeVirt plugin.

## Develop
We are willing to accept contributions. To contribute, create your own
fork, and open a pull requests against the main branch of this
repository. Make sure that your changes do not break anything by running
`make` and any relevant testing that is not already covered by unit
tests.

Note that for `make` to run properly, golangci-lint and ginkgo must be
present in the environment.
