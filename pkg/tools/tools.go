//go:build tools
// +build tools

package tools

// this version locks the tooling to a release
import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/onsi/ginkgo/v2/ginkgo"
	_ "github.com/vektra/mockery/cmd/mockery"
	_ "golang.org/x/tools/cmd/goimports"
)
