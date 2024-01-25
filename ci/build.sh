#!/bin/bash

set -e

export GO111MODULE="on"
go install github.com/mitchellh/gox@latest # cross compile
CGO_ENABLED=0 gox -os "linux darwin windows" -arch "amd64" -osarch="darwin/arm64" -osarch="darwin/arm64" -osarch="linux/arm64" -osarch="windows/386" -output "dist/{{.Dir}}_{{.OS}}_{{.Arch}}" -ldflags "-X main.buildStamp=`date -u '+%Y-%m-%d_%I:%M:%S%p'` -X main.gitRevision=`git describe --tags || git rev-parse HEAD` -s -w"
