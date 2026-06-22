BUILD_DIR ?= build
IMAGE_REGISTRY ?= quay.io/kubevirt
PUSH_REGISTRY ?= $(IMAGE_REGISTRY)
IMAGE_TAG ?= latest
SIDECAR_NAME ?= vdpa-network-binding-sidecar
WEBHOOK_NAME ?= vdpa-network-binding-admission-webhook

REQUIRE_IMAGE_PUSH_TLS_VERIFICATION ?= true

TEST_DEVICE_PLUGIN_NAME ?= vdpa-sim-net-device-plugin
TEST_CNI_NAME ?= vdpa-sim-net-cni

WEBHOOK_MANIFEST_TEMPLATE_PATH ?= $(PWD)/templates/webhook-manifest-template.yaml
WEBHOOK_MANIFEST_PATH ?= $(PWD)/manifests/vdpa-mutating-webhook.yaml
SIDECAR_MANIFEST_TEMPLATE_PATH ?= $(PWD)/templates/sidecar-patch-template.yaml
SIDECAR_MANIFEST_PATH ?= $(PWD)/manifests/vdpa-sidecar-patch.yaml
TEST_DEPENDENCIES_MANIFESTS_PATH ?= $(PWD)/test/manifests

GO_BUILD_FLAGS ?= -mod vendor
OCI_BIN ?= podman

all: lint format test build

build: build_sidecar build_admission_webhook

build_sidecar:
	go build -C sidecar $(GO_BUILD_FLAGS) -o ../$(BUILD_DIR)/$(SIDECAR_NAME)

build_admission_webhook:
	go build -C webhook $(GO_BUILD_FLAGS) -o ../$(BUILD_DIR)/$(WEBHOOK_NAME)

build_test_dependencies: build_test_cni build_test_device_plugin

build_test_device_plugin:
	go build -C test/vdpa-sim-net-device-plugin $(GO_BUILD_FLAGS) -o ../../$(BUILD_DIR)/$(TEST_DEVICE_PLUGIN_NAME)

build_test_cni:
	go build -C test/vdpa-sim-net-cni/cmd $(GO_BUILD_FLAGS) -o ../../../$(BUILD_DIR)/$(TEST_CNI_NAME)

clean:
	rm -rf $(BUILD_DIR)
	git restore manifests

format:
	@gofmt -d -s -e sidecar webhook test

format_inplace:
	gofmt -s -e -w sidecar webhook test

lint:
	golangci-lint run

test: test_sidecar test_webhook

test_sidecar:
	ginkgo -v -r sidecar

test_webhook:
	ginkgo -v -r webhook

images: image_sidecar image_webhook

image_sidecar:
	$(OCI_BIN) build -f sidecar/Containerfile -t $(IMAGE_REGISTRY)/$(SIDECAR_NAME):$(IMAGE_TAG) .

image_webhook:
	$(OCI_BIN) build -f webhook/Containerfile -t $(IMAGE_REGISTRY)/$(WEBHOOK_NAME):$(IMAGE_TAG) .

push: push_sidecar push_webhook

push_sidecar:
	$(OCI_BIN) push \
		--tls-verify=$(REQUIRE_IMAGE_PUSH_TLS_VERIFICATION) \
		$(IMAGE_REGISTRY)/$(SIDECAR_NAME):$(IMAGE_TAG) \
		$(PUSH_REGISTRY)/$(SIDECAR_NAME):$(IMAGE_TAG)

push_webhook:
	$(OCI_BIN) push \
		--tls-verify=$(REQUIRE_IMAGE_PUSH_TLS_VERIFICATION) \
		$(IMAGE_REGISTRY)/$(WEBHOOK_NAME):$(IMAGE_TAG) \
		$(PUSH_REGISTRY)/$(WEBHOOK_NAME):$(IMAGE_TAG)


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

manifests: manifest_webhook manifest_sidecar

manifest_webhook:
	@sed -e "s|VDPA_WEBHOOK_MANIFEST_TEMPLATE_IMAGE|$(IMAGE_REGISTRY)/$(WEBHOOK_NAME):$(IMAGE_TAG)|g" $(WEBHOOK_MANIFEST_TEMPLATE_PATH) > $(WEBHOOK_MANIFEST_PATH)

manifest_sidecar:
	@sed -e "s|VDPA_SIDECAR_MANIFEST_TEMPLATE_IMAGE|$(IMAGE_REGISTRY)/$(SIDECAR_NAME):$(IMAGE_TAG)|g" $(SIDECAR_MANIFEST_TEMPLATE_PATH) > $(SIDECAR_MANIFEST_PATH)

sync: sync_webhook sync_sidecar

sync_webhook: manifest_webhook
	kubectl apply -f $(WEBHOOK_MANIFEST_PATH)

sync_sidecar: manifest_sidecar
	kubectl patch -n kubevirt kubevirts kubevirt --type merge --patch-file $(SIDECAR_MANIFEST_PATH)

sync_test_dependencies:
	cat $(TEST_DEPENDENCIES_MANIFESTS_PATH)/*.yaml | \
	IMAGE_REGISTRY=$(IMAGE_REGISTRY) IMAGE_TAG=$(IMAGE_TAG) \
	TEST_DEVICE_PLUGIN_NAME=$(TEST_DEVICE_PLUGIN_NAME) \
	TEST_CNI_NAME=$(TEST_CNI_NAME) \
		envsubst | kubectl apply -f -

clean_test_dependencies:
	cat $(TEST_DEPENDENCIES_MANIFESTS_PATH)/*.yaml | \
	IMAGE_REGISTRY=$(IMAGE_REGISTRY) IMAGE_TAG=$(IMAGE_TAG) \
	TEST_DEVICE_PLUGIN_NAME=$(TEST_DEVICE_PLUGIN_NAME) \
	TEST_CNI_NAME=$(TEST_CNI_NAME) \
		envsubst | kubectl delete -f -

.PHONY: build build_sidecar build_admission_webhook build_test_device_plugin \
        clean format format_inplace lint test test_sidecar test_webhook \
        images image_sidecar image_webhook image_test_device_plugin push \
        push_sidecar push_webhook push_test_device_plugin manifests \
        manifest_webhook manifest_sidecar sync sync_webhook sync_sidecar \
        build_test_cni image_test_cni push_test_cni sync_test_dependencies \
        image_test_dependencies push_test_dependencies build_test_dependencies
