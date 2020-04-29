#!/bin/bash

export GO111MODULE="on"
go test -tags 's3 metadata' -v -race -coverprofile=coverage.txt -covermode=atomic -coverpkg github.com/aerokube/selenoid,github.com/aerokube/selenoid/session,github.com/aerokube/selenoid/config,github.com/aerokube/selenoid/protect,github.com/aerokube/selenoid/service,github.com/aerokube/selenoid/upload
