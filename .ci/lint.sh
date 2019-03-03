#!/usr/bin/env bash

set -ex

# Golint should not generate any output for a clean project.
if [[ $(golint ./...) ]]; then
    echo "Found lint errors; aborting."
    exit 1
fi

# Gofmt should not generate any output diffs for properly formatted source.
if [[ $(gofmt -s -d .) ]]; then
    echo "Found formatting errors; aborting."
    exit 1
fi
