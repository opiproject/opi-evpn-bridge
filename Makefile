# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2023 Dell Inc, or its subsidiaries.

ROOT_DIR='.'
PROJECTNAME=$(shell basename "$(PWD)")

# Make is verbose in Linux. Make it silent.
MAKEFLAGS += --silent

compile: get build

build:
	@echo "  >  Building binaries..."
	@CGO_ENABLED=0 go build -o ${PROJECTNAME} ./cmd/...

get:
	@echo "  >  Checking if there are any missing dependencies..."
	@CGO_ENABLED=0 go get ./...

tools:
	go get golang.org/x/tools/cmd/goimports
	go get github.com/kisielk/errcheck
	go get github.com/axw/gocov/gocov
	go get github.com/matm/gocov-html
	go get github.com/tools/godep
	go get github.com/mitchellh/gox
	go get github.com/golang/lint/golint

test:
	@echo "  >  Running ginkgo test suites..."
	# can replace with a recursive command ginkgo suites are defined for all packages
	ginkgo grpc pkg/evpn

vet:
	@CGO_ENABLED=0 go vet -v ./...

errors:
	errcheck -ignoretests -blank ./...

lint:
	golint ./...

imports:
	goimports -l -w .

fmt:
	@CGO_ENABLED=0 go fmt ./...

mock-generate:
	@echo "  >  Starting mock code generation..."
	# Generate mocks for exported interfaces
	mockery --config=utils/mocks/.mockery.yaml --name=Netlink --dir pkg/utils --output pkg/utils/mocks --boilerplate-file pkg/utils/mocks/boilerplate.txt --with-expecter
