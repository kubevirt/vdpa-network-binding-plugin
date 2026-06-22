#!/usr/bin/env bash
CURRENT_DIR="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
source ${CURRENT_DIR}/with_kubevirtci.sh
source ${CURRENT_DIR}/environment_variables.sh

with_kubevirtci::run ./cluster-up/ssh.sh $@
