#!/usr/bin/env sh

set -x
set -e

go version
go env

go get -t -v ./...
go build
go test -v ./...

# Cross-compile.
gox -os="linux windows" -build-toolchain
gox -os="linux windows" -output="dist/{{.Dir}}-{{.OS}}_{{.Arch}}" -parallel=2

