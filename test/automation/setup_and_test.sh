#!/usr/bin/env bash
set -xe
CURRENT_DIR="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

${CURRENT_DIR}/setup_cluster.sh
export KUBECONFIG=$(${CURRENT_DIR}/../cluster/kubeconfig.sh)
make test_integration && make cluster_down
