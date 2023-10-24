// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package models translates frontend protobuf messages to backend messages
package models

import (
	"encoding/binary"
	"net"
	"time"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

// Svi object, separate from protobuf for decoupling
type Svi struct {
	Name                string
	VrfRefKey           string
	LogicalBridgeRefKey string
	MacAddress          net.HardwareAddr
	GwIP                []net.IPNet
	EnableBgp           bool
	RemoteAs            uint32
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// build time check that struct implements interface
var _ EvpnObject[*pb.Svi] = (*Svi)(nil)

// NewSvi creates new SVI object from protobuf message
func NewSvi(in *pb.Svi) *Svi {
	mac := net.HardwareAddr(in.Spec.MacAddress)
	gwIPList := []net.IPNet{}
	for _, item := range in.Spec.GwIpPrefix {
		myip := make(net.IP, 4)
		binary.BigEndian.PutUint32(myip, item.Addr.GetV4Addr())
		gip := net.IPNet{IP: myip, Mask: net.CIDRMask(int(item.Len), 32)}
		gwIPList = append(gwIPList, gip)
	}
	svi := &Svi{
		VrfRefKey:           in.Spec.Vrf,
		LogicalBridgeRefKey: in.Spec.LogicalBridge,
		MacAddress:          mac,
		GwIP:                gwIPList,
		EnableBgp:           in.Spec.EnableBgp,
		RemoteAs:            in.Spec.RemoteAs,
		CreatedAt:           time.Now(),
	}
	return svi
}

// ToPb transforms SVI object to protobuf message
func (in *Svi) ToPb() (*pb.Svi, error) {
	svi := &pb.Svi{
		Spec: &pb.SviSpec{
			Vrf:           in.VrfRefKey,
			LogicalBridge: in.LogicalBridgeRefKey,
			MacAddress:    in.MacAddress,
			EnableBgp:     in.EnableBgp,
			RemoteAs:      in.RemoteAs,
		},
		Status: &pb.SviStatus{
			OperStatus: pb.SVIOperStatus_SVI_OPER_STATUS_UP,
		},
	}
	// TODO: add GwIpPrefix
	return svi, nil
}

// GetName returns object unique name
func (in *Svi) GetName() string {
	return in.Name
}
