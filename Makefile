LOCAL_GOPATH=${ROOT_DIR}/.gopath
BIN_DIR := .tools/bin
GOLANGCI_LINT_VERSION := 1.31.0
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint_$(GOLANGCI_LINT_VERSION)

all: build test lint

tidy:
	go mod tidy -v

build:
	go build ./...

test:
	go test -race $$(go list ./... | grep -v test) -v -coverprofile .testCoverage.txt

e2e-test:
	go test -race $$(go list ./... | grep test) -v -coverprofile .e2e.testCoverage.txt

setup: setup-git-hooks

setup-git-hooks:
	git config core.hooksPath .githooks

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --fast

$(GOLANGCI_LINT):
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BIN_DIR) v$(GOLANGCI_LINT_VERSION)
	mv $(BIN_DIR)/golangci-lint $(GOLANGCI_LINT)
