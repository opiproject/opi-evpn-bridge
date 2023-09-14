// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2023 Intel Corporation

// Package utils contains utility functions
package utils

import (
	"google.golang.org/protobuf/proto"
)

// ProtoCodec encodes/decodes Go values to/from PROTOBUF.
type ProtoCodec struct{}

// Marshal encodes a Go value to PROTOBUF.
func (c ProtoCodec) Marshal(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

// Unmarshal decodes a PROTOBUF value into a Go value.
func (c ProtoCodec) Unmarshal(data []byte, v interface{}) error {
	return proto.Unmarshal(data, v.(proto.Message))
}
