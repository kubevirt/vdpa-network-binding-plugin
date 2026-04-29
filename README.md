# vdpa-network-binding-plugin

This repository contains the source code that brings secondary [vDPA][vdpa]
network interfaces to KubeVirt.

The repository contains two components: a sidecar and a mutating
admission webhook. The sidecar is deployed as a sidecar container in the
virt-launcher pods of VMs that require a vDPA network interface. It is
in charge of mutating the domainXML to include the vDPA interface
configuration. The webhook mutates the VMI specs that contain vDPA
interfaces so the `reservedOverhead` and `memlock` configuration matches
the VM's expectations.

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

### Mutating admission webhook
A default set of manifests can be found under
`manifests/vdpa-mutating-webhook.yaml`. However, these point into the
default kubevirt image. If you built your own and pushed it into a
registry, run:
```
make manifests
```
Then apply them to your cluster
```
kubectl apply -f manifests/vdpa-mutating-webhook.yaml
```


## Develop
We are willing to accept contributions. To contribute, create your own
fork, and open a pull requests against the main branch of this
repository. Make sure that your changes do not break anything by running
`make` and any relevant testing that is not already covered by unit
tests.

Note that for `make` to run properly, golangci-lint and ginkgo must be
present in the environment.
