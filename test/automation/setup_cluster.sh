#!/usr/bin/env bash
set -xe
CURRENT_DIR="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

make kubevirtci_init
make cluster_up
make cluster_sync_kubevirt
make cluster_patch_kubevirt_featuregates
export IMAGE_REGISTRY="registry:5000"
export PUSH_REGISTRY="localhost:$(${CURRENT_DIR}/../cluster/cli.sh ports registry | tr -d '\r')"
export REQUIRE_IMAGE_PUSH_TLS_VERIFICATION=false
make image_test_dependencies
make push_test_dependencies
make sync_test_dependencies
make images
make push
make sync
${CURRENT_DIR}/../cluster/kubectl.sh rollout status daemonset vdpa-network-binding-plugin-node-plugin-hook --timeout=5m
