# KubeVirt Network vDPA Binding Plugin

## Summary

vDPA network binding plugin configures VMs vDPA interface using Kubevirts hook sidecar interface.

It will be used by Kubevirt to offload vDPA networking configuration.

# How to use

Register the `vdpa` binding plugin with its sidecar image:

```yaml
apiVersion: kubevirt.io/v1
kind: KubeVirt
metadata:
  name: kubevirt
  namespace: kubevirt
spec:
  configuration:
    network:
      binding:
        vdpa:
          sidecarImage: registry:5000/kubevirt/network-vdpa-binding:devel
  ...
```

In the VM spec, set interface to use `vdpa` binding plugin:

```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachineInstance
metadata:
  name: vmi-vdpa
spec:
  domain:
    devices:
      interfaces:
      - name: vdpa
        binding:
          name: vdpa
  ...
  networks:
  - name: vdpa-net
    pod: {}
  ...
```
