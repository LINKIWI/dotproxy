# Name of the binary executable
DOTPROXY = dotproxy

# Output binary directory
BIN_DIR = bin

# OS and architecture to use for the build
GOOS ?= $(shell go tool dist env | grep GOOS | sed 's/"//g' | sed 's/.*=//g')
GOARCH ?= $(shell go tool dist env | grep GOARCH | sed 's/"//g' | sed 's/.*=//g')

all: $(DOTPROXY)

$(DOTPROXY):
	go generate -v ./...
	go build \
		-ldflags "-X dotproxy/internal/meta.VersionSHA=$(VERSION_SHA)" \
		-o $(BIN_DIR)/$(DOTPROXY)-$(GOOS)-$(GOARCH) \
		cmd/$(DOTPROXY)/main.go

lint:
	.ci/lint.sh

clean:
	rm -f $(BIN_DIR)/*

.PHONY: lint clean
