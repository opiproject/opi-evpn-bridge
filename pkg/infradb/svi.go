// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

package infradb

import (
	"encoding/binary"
	//	"fmt"
	"log"
	"net"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	opinetcommon "github.com/opiproject/opi-api/network/opinetcommon/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
)

// SviOperStatus operational Status for SVIs
type SviOperStatus int32

const (
	// SviOperStatusUnspecified for SVI unknown state
	SviOperStatusUnspecified SviOperStatus = iota
	// SviOperStatusUp for SVI up state
	SviOperStatusUp = iota
	// SviOperStatusDown for SVI down state
	SviOperStatusDown = iota
	// SviOperStatusToBeDeleted for SVI to be deleted state
	SviOperStatusToBeDeleted = iota
)

// SviStatus holds SVI Status
type SviStatus struct {
	SviOperStatus SviOperStatus
	Components    []common.Component
}

// SviSpec holds SVI Spec
type SviSpec struct {
	Vrf           string
	LogicalBridge string
	MacAddress    *net.HardwareAddr
	// TODO: This should be plural in Protobuf as well
	GatewayIPs []*net.IPNet
	EnableBgp  bool
	RemoteAs   *uint32
}

// SviMetadata holds SVI Metadata
type SviMetadata struct {
}

// Svi holds SVI info
type Svi struct {
	Name            string
	Spec            *SviSpec
	Status          *SviStatus
	Metadata        *SviMetadata
	ResourceVersion string
}

// build time check that struct implements interface
var _ EvpnObject[*pb.Svi] = (*Svi)(nil)

// NewSvi creates new SVI object from protobuf message
func NewSvi(in *pb.Svi) *Svi {
	components := make([]common.Component, 0)
	gwIPs := make([]*net.IPNet, 0)

	// Tansform Mac From Byte to net.HardwareAddr type
	macAddr := net.HardwareAddr(in.Spec.MacAddress)

	// Parse Gateway IPs
	for _, gwIPPrefix := range in.Spec.GwIpPrefix {
		gatewayIP := make(net.IP, 4)
		binary.BigEndian.PutUint32(gatewayIP, gwIPPrefix.Addr.GetV4Addr())
		gwIP := net.IPNet{IP: gatewayIP, Mask: net.CIDRMask(int(gwIPPrefix.Len), 32)}
		gwIPs = append(gwIPs, &gwIP)
	}

	subscribers := eventbus.EBus.GetSubscribers("svi")
	if len(subscribers) == 0 {
		log.Println("NewSvi(): No subscribers for SVI objects")
	}

	for _, sub := range subscribers {
		component := common.Component{Name: sub.Name, CompStatus: common.ComponentStatusPending, Details: ""}
		components = append(components, component)
	}

	return &Svi{
		Name: in.Name,
		Spec: &SviSpec{
			Vrf:           in.Spec.Vrf,
			LogicalBridge: in.Spec.LogicalBridge,
			MacAddress:    &macAddr,
			GatewayIPs:    gwIPs,
			EnableBgp:     in.Spec.EnableBgp,
			RemoteAs:      &in.Spec.RemoteAs,
		},
		Status: &SviStatus{
			SviOperStatus: SviOperStatus(SviOperStatusDown),
			Components:    components,
		},
		Metadata:        &SviMetadata{},
		ResourceVersion: generateVersion(),
	}
}

// ToPb transforms Svi object to protobuf message
func (in *Svi) ToPb() *pb.Svi {
	gatewayIPs := make([]*opinetcommon.IPPrefix, 0)

	for _, gwIP := range in.Spec.GatewayIPs {
		gatewayIP := common.ConvertToIPPrefix(gwIP)
		gatewayIPs = append(gatewayIPs, gatewayIP)
	}

	svi := &pb.Svi{
		Name: in.Name,
		Spec: &pb.SviSpec{
			Vrf:           in.Spec.Vrf,
			LogicalBridge: in.Spec.LogicalBridge,
			MacAddress:    *in.Spec.MacAddress,
			GwIpPrefix:    gatewayIPs,
			EnableBgp:     in.Spec.EnableBgp,
			RemoteAs:      *in.Spec.RemoteAs,
		},
		Status: &pb.SviStatus{},
	}

	switch in.Status.SviOperStatus {
	case SviOperStatusDown:
		svi.Status.OperStatus = pb.SVIOperStatus_SVI_OPER_STATUS_DOWN
	case SviOperStatusUp:
		svi.Status.OperStatus = pb.SVIOperStatus_SVI_OPER_STATUS_UP
	case SviOperStatusToBeDeleted:
		svi.Status.OperStatus = pb.SVIOperStatus_SVI_OPER_STATUS_TO_BE_DELETED
	default:
		svi.Status.OperStatus = pb.SVIOperStatus_SVI_OPER_STATUS_UNSPECIFIED
	}

	for _, comp := range in.Status.Components {
		component := &pb.Component{Name: comp.Name, Details: comp.Details}

		switch comp.CompStatus {
		case common.ComponentStatusPending:
			component.Status = pb.CompStatus_COMP_STATUS_PENDING
		case common.ComponentStatusSuccess:
			component.Status = pb.CompStatus_COMP_STATUS_SUCCESS
		case common.ComponentStatusError:
			component.Status = pb.CompStatus_COMP_STATUS_ERROR
		default:
			component.Status = pb.CompStatus_COMP_STATUS_UNSPECIFIED
		}
		svi.Status.Components = append(svi.Status.Components, component)
	}

	return svi
}

// GetName returns object unique name
func (in *Svi) GetName() string {
	return in.Name
}
