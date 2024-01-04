// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2023 Nordix Foundation.

// Package utils has some utility functions and interfaces
package utils

import (
	"regexp"

	"go.einride.tech/aip/fieldmask"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ApplyMaskToStoredPbObject updates the stored PB object with the one provided
// in the update grpc request based on the provided field mask
func ApplyMaskToStoredPbObject[T proto.Message](updateMask *fieldmaskpb.FieldMask, dst, src T) {
	fieldmask.Update(updateMask, dst, src)
}

// ValidateMacAddress validates if a passing MAC address
// has the right format
func ValidateMacAddress(b []byte) error {
	macPattern := "([0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2})"
	_, err := regexp.MatchString(macPattern, string(b))
	if err != nil {
		return err
	}
	return nil
}
