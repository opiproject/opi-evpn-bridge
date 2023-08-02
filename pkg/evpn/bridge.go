// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"path"

	"github.com/vishvananda/netlink"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/resourceid"
	"go.einride.tech/aip/resourcename"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// CreateLogicalBridge executes the creation of the LogicalBridge
func (s *Server) CreateLogicalBridge(_ context.Context, in *pb.CreateLogicalBridgeRequest) (*pb.LogicalBridge, error) {
	log.Printf("CreateLogicalBridge: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.LogicalBridgeId != "" {
		err := resourceid.ValidateUserSettable(in.LogicalBridgeId)
		if err != nil {
			log.Printf("error: %v", err)
			return nil, err
		}
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.LogicalBridgeId, in.LogicalBridge.Name)
		resourceID = in.LogicalBridgeId
	}
	in.LogicalBridge.Name = resourceIDToFullName("bridges", resourceID)
	// idempotent API when called with same key, should return same object
	snet, ok := s.Bridges[in.LogicalBridge.Name]
	if ok {
		log.Printf("Already existing LogicalBridge with id %v", in.LogicalBridge.Name)
		return snet, nil
	}
	// not found, so create a new one
	bridgeName := "br-tenant"
	bridge, err := netlink.LinkByName(bridgeName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", bridgeName)
		log.Printf("error: %v", err)
		return nil, err
	}
	// Example: ip link add vxlan-<LB-vlan-id> type vxlan id <LB-vni> local <vtep-ip> dstport 4789 nolearning proxy
	vxlanName := fmt.Sprintf("vxlan%d", in.LogicalBridge.Spec.VlanId)
	myip := make(net.IP, 4)
	// TODO: remove hard-coded 167772260 == "10.0.0.100"
	binary.BigEndian.PutUint32(myip, 167772260)
	vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(in.LogicalBridge.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
	// TODO: take Port from proto instead of hard-coded
	if err := netlink.LinkAdd(vxlan); err != nil {
		fmt.Printf("Failed to create Vxlan link: %v", err)
		return nil, err
	}
	// Example: ip link set vxlan-<LB-vlan-id> master br-tenant addrgenmode none
	if err := netlink.LinkSetMaster(vxlan, bridge); err != nil {
		fmt.Printf("Failed to add Vxlan to bridge: %v", err)
		return nil, err
	}
	// Example: ip link set vxlan-<LB-vlan-id> up
	if err := netlink.LinkSetUp(vxlan); err != nil {
		fmt.Printf("Failed to up Vxlan link: %v", err)
		return nil, err
	}
	// Example: bridge vlan add dev vxlan-<LB-vlan-id> vid <LB-vlan-id> pvid untagged
	if err := netlink.BridgeVlanAdd(vxlan, uint16(in.LogicalBridge.Spec.VlanId), true, true, false, false); err != nil {
		fmt.Printf("Failed to add vlan to bridge: %v", err)
		return nil, err
	}
	// TODO: bridge link set dev vxlan-<LB-vlan-id> neigh_suppress on
	response := proto.Clone(in.LogicalBridge).(*pb.LogicalBridge)
	response.Status = &pb.LogicalBridgeStatus{OperStatus: pb.LBOperStatus_LB_OPER_STATUS_UP}
	s.Bridges[in.LogicalBridge.Name] = response
	log.Printf("CreateLogicalBridge: Sending to client: %v", response)
	return response, nil
}

// DeleteLogicalBridge deletes a LogicalBridge
func (s *Server) DeleteLogicalBridge(_ context.Context, in *pb.DeleteLogicalBridgeRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteLogicalBridge: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	obj, ok := s.Bridges[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(obj.Name)
	// use netlink to find vxlan device
	vxlan, err := netlink.LinkByName(resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		log.Printf("error: %v", err)
		return nil, err
	}
	// bring link down
	if err := netlink.LinkSetDown(vxlan); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	// delete bridge vlan
	if err := netlink.BridgeVlanDel(vxlan, uint16(obj.Spec.VlanId), true, true, false, false); err != nil {
		fmt.Printf("Failed to delete vlan to bridge: %v", err)
		return nil, err
	}
	// use netlink to delete vxlan device
	if err := netlink.LinkDel(vxlan); err != nil {
		fmt.Printf("Failed to delete link: %v", err)
		return nil, err
	}
	// remove from the Database
	delete(s.Bridges, obj.Name)
	return &emptypb.Empty{}, nil
}

// UpdateLogicalBridge updates a LogicalBridge
func (s *Server) UpdateLogicalBridge(_ context.Context, in *pb.UpdateLogicalBridgeRequest) (*pb.LogicalBridge, error) {
	log.Printf("UpdateLogicalBridge: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.LogicalBridge.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	volume, ok := s.Bridges[in.LogicalBridge.Name]
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.LogicalBridge.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.LogicalBridge); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(volume.Name)
	iface, err := netlink.LinkByName(resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		log.Printf("error: %v", err)
		return nil, err
	}
	// base := iface.Attrs()
	// iface.MTU = 1500 // TODO: remove this, just an example
	if err := netlink.LinkModify(iface); err != nil {
		fmt.Printf("Failed to update link: %v", err)
		return nil, err
	}
	// TODO: replace cloud -> evpn
	response := proto.Clone(in.LogicalBridge).(*pb.LogicalBridge)
	response.Status = &pb.LogicalBridgeStatus{OperStatus: pb.LBOperStatus_LB_OPER_STATUS_UP}
	s.Bridges[in.LogicalBridge.Name] = response
	log.Printf("UpdateLogicalBridge: Sending to client: %v", response)
	return response, nil
}

// GetLogicalBridge gets a LogicalBridge
func (s *Server) GetLogicalBridge(_ context.Context, in *pb.GetLogicalBridgeRequest) (*pb.LogicalBridge, error) {
	log.Printf("GetLogicalBridge: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	snet, ok := s.Bridges[in.Name]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(snet.Name)
	_, err := netlink.LinkByName(resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		log.Printf("error: %v", err)
		return nil, err
	}
	// TODO
	return &pb.LogicalBridge{Name: in.Name, Spec: &pb.LogicalBridgeSpec{Vni: snet.Spec.Vni, VlanId: snet.Spec.VlanId}, Status: &pb.LogicalBridgeStatus{OperStatus: pb.LBOperStatus_LB_OPER_STATUS_UP}}, nil
}
