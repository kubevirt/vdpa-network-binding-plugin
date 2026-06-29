#!/usr/bin/env bash
CURRENT_DIR="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
source ${CURRENT_DIR}/with_kubevirtci.sh
source ${CURRENT_DIR}/environment_variables.sh

if [ "$#" -ne 1 ]; then
    echo "One argument is required only" >&2
    exit 1
fi

case "$1" in
    up|down)
        action="$1"
        ;;
    *)
        echo "Error: Expected 'up' or 'down', got '$1'" >&2
        exit 1
        ;;
esac

with_kubevirtci::run make "cluster-${action}"
