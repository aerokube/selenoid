#!/bin/bash

set -e

export GO111MODULE="on"
go test -tags 's3 metadata' -v -race -coverprofile=coverage.txt -covermode=atomic -coverpkg github.com/aerokube/selenoid,github.com/aerokube/selenoid/session,github.com/aerokube/selenoid/config,github.com/aerokube/selenoid/protect,github.com/aerokube/selenoid/service,github.com/aerokube/selenoid/upload,github.com/aerokube/selenoid/info,github.com/aerokube/selenoid/jsonerror

go install golang.org/x/vuln/cmd/govulncheck@latest
"$(go env GOPATH)"/bin/govulncheck -tags production ./...
