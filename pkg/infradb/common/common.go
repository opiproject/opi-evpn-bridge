// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package common holds common functionality
package common

import (
	"net"
	"reflect"
	"time"

	pc "github.com/opiproject/opi-api/network/opinetcommon/v1alpha1/gen/go"
)

// ComponentStatus describes the status of each component
type ComponentStatus int

const (
	// ComponentStatusUnspecified for Component unknown state
	ComponentStatusUnspecified ComponentStatus = iota + 1
	// ComponentStatusPending for Component pending state
	ComponentStatusPending
	// ComponentStatusSuccess for Component success state
	ComponentStatusSuccess
	// ComponentStatusError for Component error state
	ComponentStatusError
)

// Component holds component data
type Component struct {
	Name       string
	CompStatus ComponentStatus
	// Free format json string
	Details string
	Timer   time.Duration
	// Replay is used when the module wants to trigger replay action
	Replay bool
}

// CheckReplayThreshold checks if the replay threshold has been exceeded
func (c *Component) CheckReplayThreshold(replayThreshold time.Duration) {
	c.Replay = (c.Timer > replayThreshold)
}

func ip4ToInt(ip net.IP) uint32 {
	if !reflect.ValueOf(ip).IsZero() {
		return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
	}
	return 0
}

// ConvertToIPPrefix converts IPNet type to IPPrefix
func ConvertToIPPrefix(ipNet *net.IPNet) *pc.IPPrefix {
	if ipNet == nil {
		return nil
	}

	maskLen, _ := ipNet.Mask.Size()
	return &pc.IPPrefix{
		Addr: &pc.IPAddress{
			Af: pc.IpAf_IP_AF_INET,
			V4OrV6: &pc.IPAddress_V4Addr{
				V4Addr: ip4ToInt(ipNet.IP.To4()),
			},
		},
		Len: int32(maskLen),
	}
}
