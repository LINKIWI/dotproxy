# Name of the binary executable
DOTPROXY = dotproxy

# Output binary directory
BIN_DIR = bin

# OS and architecture to use for the build
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Generated source code
GENERATED_SOURCE = internal/log/level.go \
	internal/network/server.go \
	internal/network/sharding.go
GENERATED_ARTIFACTS = internal/log/level_string.go \
	internal/network/loadbalancingpolicy_string.go \
	internal/network/transport_string.go

binary: $(DOTPROXY)

generate: $(GENERATED_ARTIFACTS)

$(DOTPROXY): $(GENERATED_ARTIFACTS)
	go build \
		-ldflags "-w -s -X dotproxy/internal/meta.VersionSHA=$(VERSION_SHA)" \
		-o $(BIN_DIR)/$(DOTPROXY)-$(GOOS)-$(GOARCH) \
		cmd/$(DOTPROXY)/main.go

$(GENERATED_ARTIFACTS): $(GENERATED_SOURCE)
	go generate -v ./...

lint:
	! gofmt -s -d . | grep "^"
	go run golang.org/x/lint/golint --set_exit_status ./...
	go vet ./...

clean:
	rm -f $(BIN_DIR)/*
	rm -f $(GENERATED_ARTIFACTS)

.PHONY: lint clean
