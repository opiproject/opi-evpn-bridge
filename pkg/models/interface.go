// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package models translates frontend protobuf messages to backend messages
package models

// EvpnObject is an interface for all domain objects in evpn-gw
type EvpnObject[T any] interface {
	ToPb() T
	GetName() string
}
