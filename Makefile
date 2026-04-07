BUILD_DIR ?= build
IMAGE_REGISTRY ?= quay.io/kubevirt
IMAGE_TAG ?= latest
SIDECAR_NAME ?= vdpa-network-binding-sidecar
WEBHOOK_NAME ?= vdpa-network-binding-admission-webhook

GO_BUILD_FLAGS ?= -mod vendor
OCI_BIN ?= podman

all: lint format test build

build: build_sidecar build_admission_webhook

build_sidecar:
	go build -C sidecar $(GO_BUILD_FLAGS) -o ../$(BUILD_DIR)/$(SIDECAR_NAME)

build_admission_webhook:
	go build -C webhook $(GO_BUILD_FLAGS) -o ../$(BUILD_DIR)/$(WEBHOOK_NAME)

clean:
	rm -rf $(BUILD_DIR)

format:
	gofmt -d -s -e sidecar webhook

format_inplace:
	gofmt -s -e -w sidecar webhook

lint:
	golint webhook sidecar

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
	$(OCI_BIN) push $(IMAGE_REGISTRY)/$(SIDECAR_NAME):$(IMAGE_TAG)

push_webhook:
	$(OCI_BIN) push $(IMAGE_REGISTRY)/$(WEBHOOK_NAME):$(IMAGE_TAG)

.PHONY: build build_sidecar build_admission_webhook clean format format_inplace \
	lint test test_sidecar test_webhook images image_sidecar image_webhook \
	push push_sidecar push_webhook
