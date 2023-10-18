// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package models translates frontend protobuf messages to backend messages
package models

import (
	// "encoding/binary"
	"net"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

// Bridge object, separate from protobuf for decoupling
type Bridge struct {
	Vni    uint32
	VlanID uint32
	VtepIP net.IPNet
}

// NewBridge creates new SVI object from protobuf message
func NewBridge(in *pb.LogicalBridge) *Bridge {
	// vtepip := make(net.IP, 4)
	// binary.BigEndian.PutUint32(vtepip, in.Spec.VtepIpPrefix.Addr.GetV4Addr())
	// vip := net.IPNet{IP: vtepip, Mask: net.CIDRMask(int(in.Spec.VtepIpPrefix.Len), 32)}
	// TODO: Vni: *in.Spec.Vni
	return &Bridge{VlanID: in.Spec.VlanId}
}

// ToPb transforms SVI object to protobuf message
func (in *Bridge) ToPb() (*pb.LogicalBridge, error) {
	bridge := &pb.LogicalBridge{
		Spec: &pb.LogicalBridgeSpec{
			Vni:    &in.Vni,
			VlanId: in.VlanID,
		},
		Status: &pb.LogicalBridgeStatus{
			OperStatus: pb.LBOperStatus_LB_OPER_STATUS_UP,
		},
	}
	// TODO: add VtepIpPrefix
	return bridge, nil
}
