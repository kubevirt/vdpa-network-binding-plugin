# vDPA Mutating Admission Webhook

A standalone Kubernetes mutating admission webhook that automatically configures memory overhead (`ReservedOverhead`) for VirtualMachines with vDPA network binding plugin interfaces.

## What it does

When a VirtualMachine with vDPA interfaces is created, this webhook:

1. Sets `MemLock` to `Required` in `spec.template.spec.domain.memory.reservedOverhead`
2. Calculates and adds `AddedOverhead` using the formula: `1Gi + (N-1) * guest_memory` for N vDPA interfaces
3. Adds the calculated overhead on top of any existing user-specified `AddedOverhead`

## Architecture

This webhook runs as an independent pod.

- On startup, generates a self-signed CA and server TLS certificate
- Creates or updates the `MutatingWebhookConfiguration` with the CA bundle
- Starts an HTTPS server to handle admission requests
