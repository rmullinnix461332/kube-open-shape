IMAGE_REPO ?= ghcr.io/kube-open-shape/edge
IMAGE_TAG  ?= latest
BIN_DIR    := bin

.PHONY: all build test lint vet clean docker-build docker-push

all: build

## Build binaries
build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN_DIR)/edge ./cmd/edge
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BIN_DIR)/kos ./cmd/kos

## Run tests
test:
	go test ./... -count=1 -race

## Run integration tests (requires live cluster + built binary)
test-integration: build
	cd test/integration && go test -v -count=1 -timeout 120s ./...

## Run helm integration tests (requires setup.sh completed + built binary)
test-helm-integration: build
	cd test/helm-integration && go test -v -count=1 -timeout 300s ./...

## Run helm integration stage 1 only (fixture charts)
test-helm-stage1: build
	cd test/helm-integration && go test -v -count=1 -timeout 120s -run TestStage1 ./...

## Run helm integration stage 2 only (application charts)
test-helm-stage2: build
	cd test/helm-integration && go test -v -count=1 -timeout 120s -run TestStage2 ./...

## Run helm integration stage 3 only (operator charts)
test-helm-stage3: build
	cd test/helm-integration && go test -v -count=1 -timeout 180s -run TestStage3 ./...

## Run organization axis edge API tests (requires edge running on :9090)
test-org-edge: build
	cd test/helm-integration && go test -v -count=1 -timeout 120s -run TestEdgeAPI ./...

## Run organization axis CLI tests (requires cluster + charts installed)
test-org-cli: build
	cd test/helm-integration && go test -v -count=1 -timeout 300s -run "TestCLI_|TestCLI" ./...

## Run go vet
vet:
	go vet ./...

## Run golangci-lint
lint:
	golangci-lint run ./...

## Tidy modules
tidy:
	go mod tidy

## Clean build artifacts
clean:
	rm -rf $(BIN_DIR)

## Build Docker image
docker-build:
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .

## Push Docker image
docker-push:
	docker push $(IMAGE_REPO):$(IMAGE_TAG)

## Generate CRDs (future)
generate:
	controller-gen crd paths="./api/..." output:crd:artifacts:config=config/crd

## Install CRDs to cluster (future)
install-crds:
	kubectl apply -f config/crd/

## Help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
