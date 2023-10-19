// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package utils has some utility functions and interfaces
package utils

import "google.golang.org/protobuf/proto"

// ProtoClone is a helper function to clone and cast protobufs
func ProtoClone[T proto.Message](protoStruct T) T {
	return proto.Clone(protoStruct).(T)
}

// EqualProtoSlices is a helper function to compare protobuf slices
func EqualProtoSlices[T proto.Message](x, y []T) bool {
	if len(x) != len(y) {
		return false
	}

	for i := 0; i < len(x); i++ {
		if !proto.Equal(x[i], y[i]) {
			return false
		}
	}

	return true
}
