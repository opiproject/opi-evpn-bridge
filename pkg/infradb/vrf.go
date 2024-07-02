// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

package infradb

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"

	// "time"
	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

// VrfOperStatus operational Status for VRFs
type VrfOperStatus int32

const (
	// VrfOperStatusUnspecified for VRF unknown state
	VrfOperStatusUnspecified VrfOperStatus = iota
	// VrfOperStatusUp for VRF up state
	VrfOperStatusUp = iota
	// VrfOperStatusDown for VRF down state
	VrfOperStatusDown = iota
	// VrfOperStatusToBeDeleted for VRF to be deleted state
	VrfOperStatusToBeDeleted = iota
)

// VrfStatus holds VRF Status
type VrfStatus struct {
	VrfOperStatus VrfOperStatus
	Components    []common.Component
}

// VrfSpec holds VRF Spec
type VrfSpec struct {
	Vni        *uint32
	LoopbackIP *net.IPNet
	VtepIP     *net.IPNet
}

// VrfMetadata holds VRF Metadata
type VrfMetadata struct {
	// We add a pointer here because the default value of uint32 type is "0"
	// and that can be considered a legit value. Using *uint32 the default value
	// will be nil
	RoutingTable []*uint32
}

// Vrf holds VRF info
type Vrf struct {
	Name            string
	Spec            *VrfSpec
	Status          *VrfStatus
	Metadata        *VrfMetadata
	Svis            map[string]bool
	ResourceVersion string
}

// build time check that struct implements interface
var _ EvpnObject[*pb.Vrf] = (*Vrf)(nil)

// NewVrfWithArgs creates a vrf object by passing arguments
func NewVrfWithArgs(name string, vni *uint32, loopbackIP, vtepIP *net.IPNet) (*Vrf, error) {
	components := make([]common.Component, 0)
	vrf := &Vrf{
		Spec:            &VrfSpec{},
		Status:          &VrfStatus{},
		Metadata:        &VrfMetadata{},
		Svis:            make(map[string]bool),
		ResourceVersion: generateVersion(),
	}

	if name == "" {
		err := fmt.Errorf("NewVrfWithArgs(): Vrf name cannot be empty")
		return nil, err
	}

	vrf.Name = name

	if vni != nil {
		vrf.Spec.Vni = vni
	}

	if loopbackIP != nil {
		vrf.Spec.LoopbackIP = loopbackIP
	}

	if vtepIP != nil {
		vrf.Spec.VtepIP = vtepIP
	}

	subscribers := eventbus.EBus.GetSubscribers("vrf")
	if len(subscribers) == 0 {
		log.Println("NewVrfWithArgs(): No subscribers for Vrf objects")
	}

	for _, sub := range subscribers {
		component := common.Component{Name: sub.Name, CompStatus: common.ComponentStatusPending, Details: ""}
		components = append(components, component)
	}

	vrf.Status = &VrfStatus{
		VrfOperStatus: VrfOperStatus(VrfOperStatusDown),
		Components:    components,
	}
	// vrf.Metadata = &VrfMetadata{}

	// vrf.Svis = make(map[string]bool)

	// vrf.ResourceVersion = generateVersion()

	return vrf, nil
}

// NewVrf creates new VRF object from protobuf message
func NewVrf(in *pb.Vrf) (*Vrf, error) {
	var vip *net.IPNet
	components := make([]common.Component, 0)

	loopip := make(net.IP, 4)
	binary.BigEndian.PutUint32(loopip, in.Spec.LoopbackIpPrefix.Addr.GetV4Addr())
	lip := net.IPNet{IP: loopip, Mask: net.CIDRMask(int(in.Spec.LoopbackIpPrefix.Len), 32)}

	// Parse vtep IP
	if in.Spec.VtepIpPrefix != nil {
		vtepip := make(net.IP, 4)
		binary.BigEndian.PutUint32(vtepip, in.Spec.VtepIpPrefix.Addr.GetV4Addr())
		vip = &net.IPNet{IP: vtepip, Mask: net.CIDRMask(int(in.Spec.VtepIpPrefix.Len), 32)}
	} else {
		tmpVtepIP := utils.GetIPAddress(config.GlobalConfig.LinuxFrr.DefaultVtep)
		vip = &tmpVtepIP
	}

	subscribers := eventbus.EBus.GetSubscribers("vrf")
	if len(subscribers) == 0 {
		log.Println("NewVrf(): No subscribers for Vrf objects")
		return &Vrf{}, errors.New("no subscribers found for vrf")
	}

	for _, sub := range subscribers {
		component := common.Component{Name: sub.Name, CompStatus: common.ComponentStatusPending, Details: ""}
		components = append(components, component)
	}

	return &Vrf{
		Name: in.Name,
		Spec: &VrfSpec{
			Vni:        in.Spec.Vni,
			LoopbackIP: &lip,
			VtepIP:     vip,
		},
		Status: &VrfStatus{
			VrfOperStatus: VrfOperStatus(VrfOperStatusDown),

			Components: components,
		},
		Metadata:        &VrfMetadata{},
		Svis:            make(map[string]bool),
		ResourceVersion: generateVersion(),
	}, nil
}

// ToPb transforms VRF object to protobuf message
func (in *Vrf) ToPb() *pb.Vrf {
	loopbackIP := common.ConvertToIPPrefix(in.Spec.LoopbackIP)
	vtepip := common.ConvertToIPPrefix(in.Spec.VtepIP)
	vrf := &pb.Vrf{
		Name: in.Name,
		Spec: &pb.VrfSpec{
			Vni:              in.Spec.Vni,
			LoopbackIpPrefix: loopbackIP,

			VtepIpPrefix: vtepip,
		},
		Status: &pb.VrfStatus{},
	}

	switch in.Status.VrfOperStatus {
	case VrfOperStatusDown:
		vrf.Status.OperStatus = pb.VRFOperStatus_VRF_OPER_STATUS_DOWN
	case VrfOperStatusUp:
		vrf.Status.OperStatus = pb.VRFOperStatus_VRF_OPER_STATUS_UP
	case VrfOperStatusToBeDeleted:
		vrf.Status.OperStatus = pb.VRFOperStatus_VRF_OPER_STATUS_TO_BE_DELETED
	default:
		vrf.Status.OperStatus = pb.VRFOperStatus_VRF_OPER_STATUS_UNSPECIFIED
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
		vrf.Status.Components = append(vrf.Status.Components, component)
	}
	return vrf
}

// AddSvi adds a reference of SVI to the VRF object
func (in *Vrf) AddSvi(sviName string) error {
	_, ok := in.Svis[sviName]
	if ok {
		return fmt.Errorf("AddSvi(): The VRF %+v is already associated with this SVI interface: %+v", in.Name, sviName)
	}

	in.Svis[sviName] = false
	return nil
}

// DeleteSvi deletes a reference of SVI from the VRF object
func (in *Vrf) DeleteSvi(sviName string) error {
	_, ok := in.Svis[sviName]
	if !ok {
		return fmt.Errorf("DeleteSvi(): The VRF %+v has no SVI interface: %+v", in.Name, sviName)
	}
	delete(in.Svis, sviName)
	return nil
}

// GetName returns object unique name
func (in *Vrf) GetName() string {
	return in.Name
}
