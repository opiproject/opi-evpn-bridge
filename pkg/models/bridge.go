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
	Name   string
	Vni    uint32
	VlanID uint32
	VtepIP net.IPNet
}

// build time check that struct implements interface
var _ EvpnObject[*pb.LogicalBridge] = (*Bridge)(nil)

func NewBridge(i *pb.LogicalBridge) EvpnObject[*pb.LogicalBridge] {
	return &Bridge{
		// vtepip := make(net.IP, 4)
		// binary.BigEndian.PutUint32(vtepip, in.Spec.VtepIpPrefix.Addr.GetV4Addr())
		// vip := net.IPNet{IP: vtepip, Mask: net.CIDRMask(int(in.Spec.VtepIpPrefix.Len), 32)}
		// TODO: Vni: *in.Spec.Vni
		VlanID: i.Spec.VlanId,
	}
}

// ToPb transforms SVI object to protobuf message
func (in *Bridge) ToPb() *pb.LogicalBridge {
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
	return bridge
}

func (in *Bridge) GetName() string {
	return in.Name
}
