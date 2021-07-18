LOCAL_GOPATH=${ROOT_DIR}/.gopath
BIN_DIR := .tools/bin
GOLANGCI_LINT_VERSION := 1.40.1
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint_$(GOLANGCI_LINT_VERSION)

.PHONY: test

build:
	go build ./...

test:
	go test -race $$(go list ./...) -v -coverprofile .testCoverage.txt

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --fast

$(GOLANGCI_LINT):
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(BIN_DIR) v$(GOLANGCI_LINT_VERSION)
	mv $(BIN_DIR)/golangci-lint $(GOLANGCI_LINT)
