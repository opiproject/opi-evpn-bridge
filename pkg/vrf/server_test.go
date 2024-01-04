// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package vrf is the main package of the application
package vrf

import (
	"testing"
)

func TestFrontEnd_NewServer(t *testing.T) {
	tests := map[string]struct{}{
		"successful call": {},
	}

	for testName := range tests {
		t.Run(testName, func(t *testing.T) {
			server := NewServer()
			if server == nil {
				t.Error("expected non nil server")
			}
		})
	}
}
