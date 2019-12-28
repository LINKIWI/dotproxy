# Name of the binary executable
DOTPROXY = dotproxy

# Output binary directory
BIN_DIR = bin

# OS and architecture to use for the build
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

all: $(DOTPROXY)

$(DOTPROXY):
	go generate -v ./...
	go build \
		-ldflags "-w -s -X dotproxy/internal/meta.VersionSHA=$(VERSION_SHA)" \
		-o $(BIN_DIR)/$(DOTPROXY)-$(GOOS)-$(GOARCH) \
		cmd/$(DOTPROXY)/main.go

lint:
	.ci/lint.sh

clean:
	rm -f $(BIN_DIR)/*

.PHONY: lint clean
