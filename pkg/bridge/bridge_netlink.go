// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package bridge is the main package of the application
package bridge

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"

	"github.com/vishvananda/netlink"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) netlinkCreateLogicalBridge(ctx context.Context, in *pb.CreateLogicalBridgeRequest) error {
	// create vxlan only if VNI is not empty
	if in.LogicalBridge.Spec.Vni != nil {
		bridge, err := s.nLink.LinkByName(ctx, tenantbridgeName)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", tenantbridgeName)
			return err
		}
		// Example: ip link add vxlan-<LB-vlan-id> type vxlan id <LB-vni> local <vtep-ip> dstport 4789 nolearning proxy
		myip := make(net.IP, 4)
		binary.BigEndian.PutUint32(myip, in.LogicalBridge.Spec.VtepIpPrefix.Addr.GetV4Addr())
		vxlanName := fmt.Sprintf("vni%d", *in.LogicalBridge.Spec.Vni)
		vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*in.LogicalBridge.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
		log.Printf("Creating Vxlan %v", vxlan)
		// TODO: take Port from proto instead of hard-coded
		if err := s.nLink.LinkAdd(ctx, vxlan); err != nil {
			fmt.Printf("Failed to create Vxlan link: %v", err)
			return err
		}
		// Example: ip link set vxlan-<LB-vlan-id> master br-tenant addrgenmode none
		if err := s.nLink.LinkSetMaster(ctx, vxlan, bridge); err != nil {
			fmt.Printf("Failed to add Vxlan to bridge: %v", err)
			return err
		}
		// Example: ip link set vxlan-<LB-vlan-id> up
		if err := s.nLink.LinkSetUp(ctx, vxlan); err != nil {
			fmt.Printf("Failed to up Vxlan link: %v", err)
			return err
		}
		// Example: bridge vlan add dev vxlan-<LB-vlan-id> vid <LB-vlan-id> pvid untagged
		if err := s.nLink.BridgeVlanAdd(ctx, vxlan, uint16(in.LogicalBridge.Spec.VlanId), true, true, false, false); err != nil {
			fmt.Printf("Failed to add vlan to bridge: %v", err)
			return err
		}
		// TODO: bridge link set dev vxlan-<LB-vlan-id> neigh_suppress on
	}
	return nil
}

func (s *Server) netlinkDeleteLogicalBridge(ctx context.Context, obj *pb.LogicalBridge) error {
	// only if VNI is not empty
	if obj.Spec.Vni != nil {
		// use netlink to find vxlan device
		vxlanName := fmt.Sprintf("vni%d", *obj.Spec.Vni)
		vxlan, err := s.nLink.LinkByName(ctx, vxlanName)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", vxlanName)
			return err
		}
		log.Printf("Deleting Vxlan %v", vxlan)
		// bring link down
		if err := s.nLink.LinkSetDown(ctx, vxlan); err != nil {
			fmt.Printf("Failed to up link: %v", err)
			return err
		}
		// delete bridge vlan
		if err := s.nLink.BridgeVlanDel(ctx, vxlan, uint16(obj.Spec.VlanId), true, true, false, false); err != nil {
			fmt.Printf("Failed to delete vlan to bridge: %v", err)
			return err
		}
		// use netlink to delete vxlan device
		if err := s.nLink.LinkDel(ctx, vxlan); err != nil {
			fmt.Printf("Failed to delete link: %v", err)
			return err
		}
	}
	return nil
}
