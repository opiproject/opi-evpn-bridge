// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package models translates frontend protobuf messages to backend messages
package models

import (
	"encoding/binary"
	"net"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

// Vrf object, separate from protobuf for decoupling
type Vrf struct {
	Vni          uint32
	LoopbackIP   net.IPNet
	VtepIP       net.IPNet
	LocalAs      int
	RoutingTable uint32
	MacAddress   net.HardwareAddr
}

// build time check that struct implements interface
var _ EvpnObject[*pb.Vrf] = (*Vrf)(nil)

// NewVrf creates new VRF object from protobuf message
func NewVrf(in *pb.Vrf) *Vrf {
	mac := net.HardwareAddr(in.Status.Rmac)
	loopip := make(net.IP, 4)
	binary.BigEndian.PutUint32(loopip, in.Spec.LoopbackIpPrefix.Addr.GetV4Addr())
	lip := net.IPNet{IP: loopip, Mask: net.CIDRMask(int(in.Spec.LoopbackIpPrefix.Len), 32)}
	// vtepip := make(net.IP, 4)
	// binary.BigEndian.PutUint32(vtepip, in.Spec.VtepIpPrefix.Addr.GetV4Addr())
	// vip := net.IPNet{IP: vtepip, Mask: net.CIDRMask(int(in.Spec.VtepIpPrefix.Len), 32)}
	return &Vrf{LoopbackIP: lip, MacAddress: mac, RoutingTable: in.Status.RoutingTable}
}

// ToPb transforms VRF object to protobuf message
func (in *Vrf) ToPb() (*pb.Vrf, error) {
	vrf := &pb.Vrf{
		Spec: &pb.VrfSpec{
			Vni: &in.Vni,
		},
		Status: &pb.VrfStatus{
			LocalAs: 4,
		},
	}
	// TODO: add LocalAs, LoopbackIP, VtepIP
	return vrf, nil
}
