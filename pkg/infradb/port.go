// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

package infradb

import (
	//	"fmt"
	"errors"

	"log"
	"net"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
)

// BridgePortType reflects the different types of a Bridge Port
type BridgePortType int32

const (
	// Unspecified bridge port type
	Unspecified BridgePortType = iota
	// Access bridge port type
	Access = iota
	// Trunk bridge port type
	Trunk = iota
)

// BridgePortOperStatus operational Status for Bridge Ports
type BridgePortOperStatus int32

const (
	// BridgePortOperStatusUnspecified for Bridge Port unknown state
	BridgePortOperStatusUnspecified BridgePortOperStatus = iota
	// BridgePortOperStatusUp for Bridge Port up state
	BridgePortOperStatusUp = iota
	// BridgePortOperStatusDown for Bridge Port down state
	BridgePortOperStatusDown = iota
	// BridgePortOperStatusToBeDeleted for Bridge Port to be deleted state
	BridgePortOperStatusToBeDeleted = iota
)

// BridgePortStatus holds Bridge Port Status
type BridgePortStatus struct {
	BPOperStatus BridgePortOperStatus
	Components   []common.Component
}

// BridgePortSpec holds Bridge Port Spec
type BridgePortSpec struct {
	Name           string
	Ptype          BridgePortType
	MacAddress     *net.HardwareAddr
	LogicalBridges []string
}

// BridgePortMetadata holds Bridge Port Metadata
type BridgePortMetadata struct {
	// Dimitris: We assume that this is Vendor specific
	// so it will be generated by the LVM
	VPort string
}

// BridgePort holds Bridge Port info
type BridgePort struct {
	Domain
	Name             string
	Spec             *BridgePortSpec
	Status           *BridgePortStatus
	Metadata         *BridgePortMetadata
	TransparentTrunk bool
	Vlans            []*uint32
	ResourceVersion  string
}

// build time check that struct implements interface
var _ EvpnObject[*pb.BridgePort] = (*BridgePort)(nil)

// NewBridgePort creates new Bridge Port object from protobuf message
func NewBridgePort(in *pb.BridgePort) (*BridgePort, error) {
	var bpType BridgePortType
	var transTrunk bool
	components := make([]common.Component, 0)

	// Tansform Mac From Byte to net.HardwareAddr type
	macAddr := net.HardwareAddr(in.Spec.MacAddress)

	subscribers := eventbus.EBus.GetSubscribers("bridge-port")
	if len(subscribers) == 0 {
		log.Println("NewBridgePort(): No subscribers for Bridge Port objects")
		return &BridgePort{}, errors.New("no subscribers found for bridge port")
	}

	for _, sub := range subscribers {
		component := common.Component{Name: sub.Name, CompStatus: common.ComponentStatusPending, Details: ""}
		components = append(components, component)
	}

	if len(in.Spec.LogicalBridges) == 0 {
		transTrunk = true
	}

	switch in.Spec.Ptype {
	case pb.BridgePortType_BRIDGE_PORT_TYPE_ACCESS:
		bpType = Access
	case pb.BridgePortType_BRIDGE_PORT_TYPE_TRUNK:
		bpType = Trunk
	default:
		bpType = Unspecified
	}

	return &BridgePort{
		Name: in.Name,
		Spec: &BridgePortSpec{
			Ptype:          bpType,
			MacAddress:     &macAddr,
			LogicalBridges: in.Spec.LogicalBridges,
		},
		Status: &BridgePortStatus{
			BPOperStatus: BridgePortOperStatus(BridgePortOperStatusDown),
			Components:   components,
		},
		Metadata:         &BridgePortMetadata{},
		TransparentTrunk: transTrunk,
		ResourceVersion:  generateVersion(),
	}, nil
}

// ToPb transforms Bridge Port object to protobuf message
func (in *BridgePort) ToPb() *pb.BridgePort {
	bp := &pb.BridgePort{
		Name: in.Name,
		Spec: &pb.BridgePortSpec{
			MacAddress: *in.Spec.MacAddress,
		},
		Status: &pb.BridgePortStatus{},
	}

	switch in.Spec.Ptype {
	case Access:
		bp.Spec.Ptype = pb.BridgePortType_BRIDGE_PORT_TYPE_ACCESS
	case Trunk:
		bp.Spec.Ptype = pb.BridgePortType_BRIDGE_PORT_TYPE_TRUNK
	default:
		bp.Spec.Ptype = pb.BridgePortType_BRIDGE_PORT_TYPE_UNSPECIFIED
	}

	if !in.TransparentTrunk {
		bp.Spec.LogicalBridges = in.Spec.LogicalBridges
	}

	switch in.Status.BPOperStatus {
	case BridgePortOperStatusDown:
		bp.Status.OperStatus = pb.BPOperStatus_BP_OPER_STATUS_DOWN
	case BridgePortOperStatusUp:
		bp.Status.OperStatus = pb.BPOperStatus_BP_OPER_STATUS_UP
	case BridgePortOperStatusToBeDeleted:
		bp.Status.OperStatus = pb.BPOperStatus_BP_OPER_STATUS_TO_BE_DELETED
	default:
		bp.Status.OperStatus = pb.BPOperStatus_BP_OPER_STATUS_UNSPECIFIED
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
		bp.Status.Components = append(bp.Status.Components, component)
	}

	return bp
}

// GetName returns object unique name
func (in *BridgePort) GetName() string {
	return in.Name
}

// setComponentState set the stat of the component
func (in *BridgePort) setComponentState(component common.Component) {
	bpComponents := in.Status.Components
	for i, comp := range bpComponents {
		if comp.Name == component.Name {
			in.Status.Components[i] = component
			break
		}
	}
}

// checkForAllSuccess check if all the components are in Success state
func (in *BridgePort) checkForAllSuccess() bool {
	for _, comp := range in.Status.Components {
		if comp.CompStatus != common.ComponentStatusSuccess {
			return false
		}
	}
	return true
}

// parseMeta parse metadata
func (in *BridgePort) parseMeta(bpMeta *BridgePortMetadata) {
	if bpMeta != nil {
		if bpMeta.VPort != "" {
			in.Metadata.VPort = bpMeta.VPort
		}
	}
}

func (in *BridgePort) getStatusComponents() []common.Component {
	return in.Status.Components
}

func (in *BridgePort) setStatusComponents(components []common.Component) {
	copy(in.Status.Components, components)
}

func (in *BridgePort) isOperationalStatus(operStatus OperStatus) bool {
	switch operStatus {
	case OperStatusUp:
		return in.Status.BPOperStatus == BridgePortOperStatusUp
	case OperStatusDown:
		return in.Status.BPOperStatus == BridgePortOperStatusDown
	case OperStatusToBeDeleted:
		return in.Status.BPOperStatus == BridgePortOperStatusToBeDeleted
	case OperStatusUnspecified:
		return in.Status.BPOperStatus == BridgePortOperStatusUnspecified
	default:
		log.Println("isOperationalStatus(): operational status has not been identified")
		return false
	}
}

func (in *BridgePort) setOperationalStatus(operStatus OperStatus) {
	switch operStatus {
	case OperStatusUp:
		in.Status.BPOperStatus = BridgePortOperStatusUp
	case OperStatusDown:
		in.Status.BPOperStatus = BridgePortOperStatusDown
	case OperStatusToBeDeleted:
		in.Status.BPOperStatus = BridgePortOperStatusToBeDeleted
	case OperStatusUnspecified:
		in.Status.BPOperStatus = BridgePortOperStatusUnspecified
	default:
		log.Println("setOperationalStatus(): operational status has not been identified")
	}
}

// TODO: This function can probably be moved to the domain.go as the ResourceVersion
// field is common for all the child objects (VRF,LB, BP, SVI)
func (in *BridgePort) setNewResourceVersion() {
	in.ResourceVersion = generateVersion()
}
