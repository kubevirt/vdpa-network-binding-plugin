#!/usr/bin/env bash
CURRENT_DIR="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
source ${CURRENT_DIR}/with_kubevirtci.sh

export KUBEVIRT_PROVIDER=${KUBEVIRT_PROVIDER:-"k8s-1.35"}
export KUBEVIRT_NUM_NODES=${KUBEVIRT_NUM_NODES:-"2"}
export KUBEVIRT_WITH_MULTUS=true
export KUBEVIRT_DEPLOY_NETWORK_RESOURCES_INJECTOR=true
if [ ! -z KUBEVIRTCI_TAG ]; then
    export KUBEVIRTCI_TAG="$(with_kubevirtci::run git describe --tags --exact-match)"
    if [ -z KUBEVIRTCI_TAG ]; then
        echo "Can't find KUBEVIRTCI_TAG automatically. Provide one."
        exit 1
    fi
fi
