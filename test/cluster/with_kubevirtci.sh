#!/usr/bin/env bash
CURRENT_DIR="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
KUBEVIRTCI_DIR="$( cd -- ${CURRENT_DIR}/../kubevirtci ; pwd -P)"

with_kubevirtci::run() {
    cd ${KUBEVIRTCI_DIR} && exec "$@"
}
