// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

package infradb

import (
	// "encoding/binary"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

// LogicalBridgeOperStatus operational Status for Logical Bridges
type LogicalBridgeOperStatus int32

const (
	// LogicalBridgeOperStatusUnspecified for Logical Bridge unknown state
	LogicalBridgeOperStatusUnspecified LogicalBridgeOperStatus = iota
	// LogicalBridgeOperStatusUp for Logical Bridge up state
	LogicalBridgeOperStatusUp = iota
	// LogicalBridgeOperStatusDown for Logical Bridge down state
	LogicalBridgeOperStatusDown = iota
	// LogicalBridgeOperStatusToBeDeleted for Logical Bridge to be deleted state
	LogicalBridgeOperStatusToBeDeleted = iota
)

// LogicalBridgeStatus holds Logical Bridge Status
type LogicalBridgeStatus struct {
	LBOperStatus LogicalBridgeOperStatus
	Components   []common.Component
}

// LogicalBridgeSpec holds Logical Bridge Spec
type LogicalBridgeSpec struct {
	VlanID uint32
	Vni    *uint32
	VtepIP *net.IPNet
}

// LogicalBridgeMetadata holds Logical Bridge Metadata
type LogicalBridgeMetadata struct{}

// LogicalBridge holds Logical Bridge info
type LogicalBridge struct {
	Domain
	Name            string
	Spec            *LogicalBridgeSpec
	Status          *LogicalBridgeStatus
	Metadata        *LogicalBridgeMetadata
	Svi             string
	BridgePorts     map[string]bool
	MacTable        map[string]string
	ResourceVersion string
}

// build time check that struct implements interface
var _ EvpnObject[*pb.LogicalBridge] = (*LogicalBridge)(nil)

// NewLogicalBridge creates new Logical Bridge object from protobuf message
func NewLogicalBridge(in *pb.LogicalBridge) (*LogicalBridge, error) {
	var vip *net.IPNet
	components := make([]common.Component, 0)

	// Parse vtep IP
	if in.Spec.VtepIpPrefix != nil {
		vtepip := make(net.IP, 4)
		binary.BigEndian.PutUint32(vtepip, in.Spec.VtepIpPrefix.Addr.GetV4Addr())
		vip = &net.IPNet{IP: vtepip, Mask: net.CIDRMask(int(in.Spec.VtepIpPrefix.Len), 32)}
	} else {
		tmpVtepIP := utils.GetIPAddress(config.GlobalConfig.LinuxFrr.DefaultVtep)
		vip = &tmpVtepIP
	}

	subscribers := eventbus.EBus.GetSubscribers("logical-bridge")
	if len(subscribers) == 0 {
		log.Println("NewLogicalBridge(): No subscribers for Logical Bridge objects")
		return &LogicalBridge{}, errors.New("no subscribers found for logical bridge")
	}

	for _, sub := range subscribers {
		component := common.Component{Name: sub.Name, CompStatus: common.ComponentStatusPending, Details: ""}
		components = append(components, component)
	}

	return &LogicalBridge{
		Name: in.Name,
		Spec: &LogicalBridgeSpec{
			VlanID: in.Spec.VlanId,
			Vni:    in.Spec.Vni,
			VtepIP: vip,
		},
		Status: &LogicalBridgeStatus{
			LBOperStatus: LogicalBridgeOperStatus(LogicalBridgeOperStatusDown),
			Components:   components,
		},
		Metadata:        &LogicalBridgeMetadata{},
		BridgePorts:     make(map[string]bool),
		MacTable:        make(map[string]string),
		ResourceVersion: generateVersion(),
	}, nil
}

// ToPb transforms Logical Bridge object to protobuf message
func (in *LogicalBridge) ToPb() *pb.LogicalBridge {
	vtepip := common.ConvertToIPPrefix(in.Spec.VtepIP)

	lb := &pb.LogicalBridge{
		Name: in.Name,
		Spec: &pb.LogicalBridgeSpec{
			VlanId:       in.Spec.VlanID,
			Vni:          in.Spec.Vni,
			VtepIpPrefix: vtepip,
		},
		Status: &pb.LogicalBridgeStatus{},
	}

	switch in.Status.LBOperStatus {
	case LogicalBridgeOperStatusDown:
		lb.Status.OperStatus = pb.LBOperStatus_LB_OPER_STATUS_DOWN
	case LogicalBridgeOperStatusUp:
		lb.Status.OperStatus = pb.LBOperStatus_LB_OPER_STATUS_UP
	case LogicalBridgeOperStatusToBeDeleted:
		lb.Status.OperStatus = pb.LBOperStatus_LB_OPER_STATUS_TO_BE_DELETED
	default:
		lb.Status.OperStatus = pb.LBOperStatus_LB_OPER_STATUS_UNSPECIFIED
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
		lb.Status.Components = append(lb.Status.Components, component)
	}

	return lb
}

// AddSvi adds a reference of SVI to the Logical Bridge object
func (in *LogicalBridge) AddSvi(sviName string) error {
	if in.Svi != "" {
		return fmt.Errorf("AddSvi(): the logical bridge is already associated with an svi interface: %+v", in.Svi)
	}

	in.Svi = sviName
	return nil
}

// DeleteSvi deletes a reference of SVI from the Logical Bridge object
func (in *LogicalBridge) DeleteSvi(sviName string) error {
	if in.Svi != sviName {
		return fmt.Errorf("DeleteSvi(): the logical bridge is not associated with the svi interface: %+v", sviName)
	}

	in.Svi = ""
	return nil
}

// AddBridgePort adds a reference of a Bridge Port to the Logical Bridge object
func (in *LogicalBridge) AddBridgePort(bpName, bpMac string) error {
	_, found := in.BridgePorts[bpName]
	if found {
		return fmt.Errorf("AddBridgePort(): the logical bridge %+v is already associated with the bridge port: %+v", in.Name, bpName)
	}

	_, found = in.MacTable[bpMac]
	if found {
		return fmt.Errorf("AddBridgePort(): the logical bridge %+v is already associated with the bridge port mac: %+v", in.Name, bpMac)
	}
	in.BridgePorts[bpName] = false
	in.MacTable[bpMac] = bpName

	return nil
}

// DeleteBridgePort deletes a reference of a Bridge Port from the Logical Bridge object
func (in *LogicalBridge) DeleteBridgePort(bpName, bpMac string) error {
	_, found := in.BridgePorts[bpName]
	if !found {
		return fmt.Errorf("DeleteBridgePort(): the logical bridge %+v is not associated with the bridge port: %+v", in.Name, bpName)
	}

	_, found = in.MacTable[bpMac]
	if !found {
		return fmt.Errorf("DeleteBridgePort(): the logical bridge %+v is not associated with the bridge port mac: %+v", in.Name, bpMac)
	}

	delete(in.BridgePorts, bpName)
	delete(in.MacTable, bpMac)

	return nil
}

// GetName returns object unique name
func (in *LogicalBridge) GetName() string {
	return in.Name
}

// setComponentState set the stat of the component
func (in *LogicalBridge) setComponentState(component common.Component) {
	lbComponents := in.Status.Components
	for i, comp := range lbComponents {
		if comp.Name == component.Name {
			in.Status.Components[i] = component
			break
		}
	}
}

// checkForAllSuccess check if all the components are in Success state
func (in *LogicalBridge) checkForAllSuccess() bool {
	for _, comp := range in.Status.Components {
		if comp.CompStatus != common.ComponentStatusSuccess {
			return false
		}
	}
	return true
}

// parseMeta parse metadata
func (in *LogicalBridge) parseMeta(lbMeta *LogicalBridgeMetadata) {
	if lbMeta != nil {
		in.Metadata = lbMeta
	}
}

func (in *LogicalBridge) getStatusComponents() []common.Component {
	return in.Status.Components
}

func (in *LogicalBridge) setStatusComponents(components []common.Component) {
	copy(in.Status.Components, components)
}

func (in *LogicalBridge) isOperationalStatus(operStatus OperStatus) bool {
	switch operStatus {
	case OperStatusUp:
		return in.Status.LBOperStatus == LogicalBridgeOperStatusUp
	case OperStatusDown:
		return in.Status.LBOperStatus == LogicalBridgeOperStatusDown
	case OperStatusToBeDeleted:
		return in.Status.LBOperStatus == LogicalBridgeOperStatusToBeDeleted
	case OperStatusUnspecified:
		return in.Status.LBOperStatus == LogicalBridgeOperStatusUnspecified
	default:
		log.Println("isOperationalStatus(): operational status has not been identified")
		return false
	}
}

func (in *LogicalBridge) setOperationalStatus(operStatus OperStatus) {
	switch operStatus {
	case OperStatusUp:
		in.Status.LBOperStatus = LogicalBridgeOperStatusUp
	case OperStatusDown:
		in.Status.LBOperStatus = LogicalBridgeOperStatusDown
	case OperStatusToBeDeleted:
		in.Status.LBOperStatus = LogicalBridgeOperStatusToBeDeleted
	case OperStatusUnspecified:
		in.Status.LBOperStatus = LogicalBridgeOperStatusUnspecified
	default:
		log.Println("setOperationalStatus(): operational status has not been identified")
	}
}

// TODO: This function can probably be moved to the domain.go as the ResourceVersion
// field is common for all the child objects (VRF,LB, BP, SVI)
func (in *LogicalBridge) setNewResourceVersion() {
	in.ResourceVersion = generateVersion()
}
