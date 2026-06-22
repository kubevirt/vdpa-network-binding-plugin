#!/usr/bin/env bash
CURRENT_DIR="$( cd -- "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"
source ${CURRENT_DIR}/environment_variables.sh

KUBEVIRT_BUCKET_BASE="https://storage.googleapis.com/kubevirt-prow"
KUBEVIRT_BUCKET_NIGHTLY_DIR="${KUBEVIRT_BUCKET_BASE}/devel/nightly/release/kubevirt/kubevirt"
KUBEVIRT_BUCKET_NIGHTLY_LATEST_POINTER="${KUBEVIRT_BUCKET_NIGHTLY_DIR}/latest"
KUBEVIRT_BUCKET_RELEASES_LATEST_POINTER="${KUBEVIRT_BUCKET_BASE}/release/kubevirt/kubevirt/stable.txt"
KUBEVIRT_RELEASES_DOWNLOAD_DIR="https://github.com/kubevirt/kubevirt/releases/download"

KUBEVIRT_OPERATOR_MANIFEST="kubevirt-operator.yaml"
KUBEVIRT_CR_MANIFEST="kubevirt-cr.yaml"

_kubectl() {
    ${CURRENT_DIR}/kubectl.sh $@
}

latest_nightly_kubevirt_date() {
    curl -L ${KUBEVIRT_BUCKET_NIGHTLY_LATEST_POINTER}
}

latest_nightly_kubevirt_dir() {
    latest="$1"
    echo ${KUBEVIRT_BUCKET_NIGHTLY_DIR}/${latest}
}

latest_nightly_manifest() {
    latest="$1"
    manifest="$2"

    echo "$(latest_nightly_kubevirt_dir $1)"/${manifest}
}

latest_released_version() {
    curl -L ${KUBEVIRT_BUCKET_RELEASES_LATEST_POINTER}
}

release_manifest() {
    version="$1"
    manifest="$2"

    echo ${KUBEVIRT_RELEASES_DOWNLOAD_DIR}/${version}/${manifest}
}

semver_sanity_check() {
    version="$1"
    echo ${version} | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc).[0-9]+)?$'
}

install_kubevirt() {
    operator_manifest="$1"
    cr_manifest="$2"

    _kubectl apply -f ${operator_manifest}
    _kubectl apply -f ${cr_manifest}
    _kubectl wait -n kubevirt kubevirts kubevirt --for condition=Available --timeout 15m
}

main() {
    if [ "$#" -ne 1 ]; then
        echo "just one argument is required" >&2
        exit 1
    fi

    target_version="$1"
    kubevirt_operator_manifest=""
    kubevirt_cr_manifest=""
    case ${target_version} in
        nightly)
            latest_nightly="$(latest_nightly_kubevirt_date)"
            kubevirt_operator_manifest="$(latest_nightly_manifest ${latest_nightly} ${KUBEVIRT_OPERATOR_MANIFEST})"
            kubevirt_cr_manifest="$(latest_nightly_manifest ${latest_nightly} ${KUBEVIRT_CR_MANIFEST})"
            ;;
        latest)
            latest_release="$(latest_released_version)"
            kubevirt_operator_manifest="$(release_manifest ${latest_release} ${KUBEVIRT_OPERATOR_MANIFEST})"
            kubevirt_cr_manifest="$(release_manifest ${latest_release} ${KUBEVIRT_CR_MANIFEST})"
            ;;
        *)
            if ! semver_sanity_check ${target_version}; then
                echo "${target_version} is not a valid version identifier. Start trying 'latest' or 'nightly'."
                exit 1
            fi
            kubevirt_operator_manifest="$(release_manifest ${target_version} ${KUBEVIRT_OPERATOR_MANIFEST})"
            kubevirt_cr_manifest="$(release_manifest ${target_version} ${KUBEVIRT_CR_MANIFEST})"
            ;;
    esac

    install_kubevirt ${kubevirt_operator_manifest} ${kubevirt_cr_manifest}
}

set -e
main $@
