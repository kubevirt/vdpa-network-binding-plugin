BUILD_DIR ?= build
IMAGE_REGISTRY ?= quay.io/kubevirt
PUSH_REGISTRY ?= $(IMAGE_REGISTRY)
IMAGE_TAG ?= latest
SIDECAR_NAME ?= vdpa-network-binding-sidecar
NODEHOOK_NAME ?= vdpa-network-binding-node-hook
VDPA_SIDECAR_IMAGE = $(IMAGE_REGISTRY)/$(SIDECAR_NAME):$(IMAGE_TAG)
VDPA_NODEHOOK_IMAGE = $(IMAGE_REGISTRY)/$(NODEHOOK_NAME):$(IMAGE_TAG)
VDPA_NETWORK_BINDING_PLUGIN_NAME ?= vdpa

REQUIRE_IMAGE_PUSH_TLS_VERIFICATION ?= true

TEST_KUBEVIRTCI_PATH = ./test/kubevirtci
TEST_DEVICE_PLUGIN_NAME ?= vdpa-sim-net-device-plugin
TEST_CNI_NAME ?= vdpa-sim-net-cni

SIDECAR_MANIFEST_TEMPLATE_PATH ?= $(PWD)/templates/sidecar-patch-template.yaml
SIDECAR_MANIFEST_PATH ?= $(PWD)/manifests/vdpa-sidecar-patch.yaml
NODE_HOOK_MANIFEST_TEMPLATE_PATH ?= $(PWD)/templates/node-hook-template.yaml
NODE_HOOK_MANIFEST_PATH ?= $(PWD)/manifests/vdpa-node-hook.yaml
TEST_DEPENDENCIES_MANIFESTS_PATH ?= $(PWD)/test/manifests
KUBEVIRT_SYNC_VERSION ?= latest

GO_BUILD_FLAGS ?= -mod vendor
OCI_BIN ?= podman

all: lint format test build

build: build_sidecar build_nodehook

build_sidecar:
	go build -C sidecar $(GO_BUILD_FLAGS) -o ../$(BUILD_DIR)/$(SIDECAR_NAME)

build_nodehook:
	CGO_ENABLED=0 go build -C nodehook $(GO_BUILD_FLAGS) -o ../$(BUILD_DIR)/$(NODEHOOK_NAME)

build_test_dependencies: build_test_cni build_test_device_plugin

build_test_device_plugin:
	go build -C test/vdpa-sim-net-device-plugin $(GO_BUILD_FLAGS) -o ../../$(BUILD_DIR)/$(TEST_DEVICE_PLUGIN_NAME)

build_test_cni:
	go build -C test/vdpa-sim-net-cni/cmd $(GO_BUILD_FLAGS) -o ../../../$(BUILD_DIR)/$(TEST_CNI_NAME)

clean:
	rm -rf $(BUILD_DIR)
	git restore manifests

format:
	@gofmt -d -s -e sidecar nodehook test

format_inplace:
	gofmt -s -e -w sidecar nodehook test

lint:
	golangci-lint run

test: test_sidecar test_nodehook

test_sidecar:
	ginkgo -v -r sidecar

test_nodehook:
	ginkgo -v -r nodehook

generate:
	find . -type d -name vendor -prune -o -name kubevirtci -type d -prune -o -type f -name "*.go" -exec go generate {} \;

images: image_sidecar image_nodehook

image_sidecar:
	$(OCI_BIN) build -f sidecar/Containerfile -t $(VDPA_SIDECAR_IMAGE) .

image_nodehook:
	$(OCI_BIN) build -f nodehook/Containerfile -t $(VDPA_NODEHOOK_IMAGE) .

push: push_sidecar push_nodehook

push_sidecar:
	$(OCI_BIN) push \
		--tls-verify=$(REQUIRE_IMAGE_PUSH_TLS_VERIFICATION) \
		$(VDPA_SIDECAR_IMAGE) \
		$(PUSH_REGISTRY)/$(SIDECAR_NAME):$(IMAGE_TAG)

push_nodehook:
	$(OCI_BIN) push \
		--tls-verify=$(REQUIRE_IMAGE_PUSH_TLS_VERIFICATION) \
		$(VDPA_NODEHOOK_IMAGE) \
		$(PUSH_REGISTRY)/$(NODEHOOK_NAME):$(IMAGE_TAG)


image_test_dependencies: image_test_device_plugin image_test_cni
push_test_dependencies: push_test_device_plugin push_test_cni

image_test_device_plugin:
	$(OCI_BIN) build -f test/vdpa-sim-net-device-plugin/Containerfile -t $(IMAGE_REGISTRY)/$(TEST_DEVICE_PLUGIN_NAME):$(IMAGE_TAG) .

push_test_device_plugin:
	$(OCI_BIN) push \
		--tls-verify=$(REQUIRE_IMAGE_PUSH_TLS_VERIFICATION) \
		$(IMAGE_REGISTRY)/$(TEST_DEVICE_PLUGIN_NAME):$(IMAGE_TAG) \
		$(PUSH_REGISTRY)/$(TEST_DEVICE_PLUGIN_NAME):$(IMAGE_TAG)

image_test_cni:
	$(OCI_BIN) build -f test/vdpa-sim-net-cni/Containerfile -t $(IMAGE_REGISTRY)/$(TEST_CNI_NAME):$(IMAGE_TAG) .

push_test_cni:
	$(OCI_BIN) push \
		--tls-verify=$(REQUIRE_IMAGE_PUSH_TLS_VERIFICATION) \
		$(IMAGE_REGISTRY)/$(TEST_CNI_NAME):$(IMAGE_TAG) \
		$(PUSH_REGISTRY)/$(TEST_CNI_NAME):$(IMAGE_TAG)

manifests: manifest_sidecar manifest_nodehook

manifest_sidecar:
	@cat $(SIDECAR_MANIFEST_TEMPLATE_PATH) | \
		VDPA_SIDECAR_IMAGE=$(VDPA_SIDECAR_IMAGE) \
		VDPA_NETWORK_BINDING_PLUGIN_NAME=$(VDPA_NETWORK_BINDING_PLUGIN_NAME) \
		envsubst > $(SIDECAR_MANIFEST_PATH)

manifest_nodehook:
	@cat $(NODE_HOOK_MANIFEST_TEMPLATE_PATH) | \
		VDPA_NODEHOOK_IMAGE=$(VDPA_NODEHOOK_IMAGE) \
		VDPA_NETWORK_BINDING_PLUGIN_NAME=$(VDPA_NETWORK_BINDING_PLUGIN_NAME) \
		envsubst > $(NODE_HOOK_MANIFEST_PATH)

sync: sync_sidecar sync_nodehook

sync_sidecar: manifest_sidecar
	./test/cluster/kubectl.sh patch -n kubevirt kubevirts kubevirt --type merge --patch-file $(SIDECAR_MANIFEST_PATH)

sync_nodehook: manifest_nodehook
	./test/cluster/kubectl.sh apply -f $(NODE_HOOK_MANIFEST_PATH)

sync_test_dependencies:
	cat $(TEST_DEPENDENCIES_MANIFESTS_PATH)/*.yaml | \
	IMAGE_REGISTRY=$(IMAGE_REGISTRY) IMAGE_TAG=$(IMAGE_TAG) \
	TEST_DEVICE_PLUGIN_NAME=$(TEST_DEVICE_PLUGIN_NAME) \
	TEST_CNI_NAME=$(TEST_CNI_NAME) \
		envsubst | ./test/cluster/kubectl.sh apply -f -

clean_test_dependencies:
	cat $(TEST_DEPENDENCIES_MANIFESTS_PATH)/*.yaml | \
	IMAGE_REGISTRY=$(IMAGE_REGISTRY) IMAGE_TAG=$(IMAGE_TAG) \
	TEST_DEVICE_PLUGIN_NAME=$(TEST_DEVICE_PLUGIN_NAME) \
	TEST_CNI_NAME=$(TEST_CNI_NAME) \
		envsubst | ./test/cluster/kubectl.sh delete -f -

kubevirtci_init:
	git submodule update --init --recursive -- $(TEST_KUBEVIRTCI_PATH)

kubevirtci_update:
	git submodule update --remote --rebase -- $(TEST_KUBEVIRTCI_PATH)

cluster_up:
	./test/cluster/cluster.sh up

cluster_down:
	./test/cluster/cluster.sh down

cluster_sync_kubevirt:
	./test/cluster/install_kubevirt.sh ${KUBEVIRT_SYNC_VERSION}

cluster_patch_kubevirt_featuregates:
	./test/cluster/kubectl.sh patch kubevirt kubevirt -n kubevirt --type='merge' -p '{"spec":{"configuration":{"developerConfiguration":{"featureGates":["Plugins"]}}}}'

test_integration:
	go test -C test/integration/ -kubeconfig=${KUBECONFIG} --ginkgo.vv

.PHONY: build \
	build_sidecar \
	build_test_device_plugin \
	clean \
	format \
	format_inplace \
	lint \
	test \
	test_sidecar \
	images \
	image_sidecar \
	image_test_device_plugin \
	push \
	push_sidecar \
	push_test_device_plugin \
	manifests \
	manifest_sidecar \
	sync \
	sync_sidecar \
	build_test_cni \
	image_test_cni \
	push_test_cni \
	sync_test_dependencies \
	image_test_dependencies \
	push_test_dependencies \
	build_test_dependencies \
	kubevirtci_init \
	kubevirtci_update \
	cluster_up \
	cluster_down \
	cluster_sync_kubevirt \
	test_integration \
	generate \
	build_nodehook \
	test_nodehook \
	image_nodehook \
	push_nodehook \
	manifest_nodehook \
	sync_nodehook \
	cluster_patch_kubevirt_featuregates
