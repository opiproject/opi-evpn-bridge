// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package repository is the database abstraction implementing repository design pattern
package repository

import (
	"encoding/binary"
	"net"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

// Vrf object, separate from protobuf for decoupling
type Vrf struct {
	Vni        uint32
	LoopbackIP net.IPNet
	VtepIP     net.IPNet
}

// NewVrf creates new VRF object from protobuf message
func NewVrf(in *pb.Vrf) *Vrf {
	loopip := make(net.IP, 4)
	binary.BigEndian.PutUint32(loopip, in.Spec.LoopbackIpPrefix.Addr.GetV4Addr())
	lip := net.IPNet{IP: loopip, Mask: net.CIDRMask(int(in.Spec.LoopbackIpPrefix.Len), 32)}
	vtepip := make(net.IP, 4)
	binary.BigEndian.PutUint32(vtepip, in.Spec.VtepIpPrefix.Addr.GetV4Addr())
	vip := net.IPNet{IP: vtepip, Mask: net.CIDRMask(int(in.Spec.VtepIpPrefix.Len), 32)}
	return &Vrf{Vni: *in.Spec.Vni, LoopbackIP: lip, VtepIP: vip}
}

// ToPb transforms VRF object to protobuf message
func (in *Vrf) ToPb() (*pb.Vrf, error) {
	return &pb.Vrf{Spec: nil, Status: nil}, nil
}
