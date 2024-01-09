// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package port is the main package of the application
package port

import (
	"context"
	"fmt"
	"path"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) netlinkCreateBridgePort(ctx context.Context, in *pb.CreateBridgePortRequest) error {
	resourceID := path.Base(in.BridgePort.Name)
	// use netlink to find br-tenant
	bridge, err := s.nLink.LinkByName(ctx, tenantbridgeName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", tenantbridgeName)
		return err
	}
	// get base interface (e.g.: eth2)
	iface, err := s.nLink.LinkByName(ctx, resourceID)
	// TODO: maybe we need to create a new iface here and not rely on existing one ?
	//		 iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: resourceID}}
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		return err
	}
	// Example: ip link set eth2 addr aa:bb:cc:00:00:41
	if len(in.BridgePort.Spec.MacAddress) > 0 {
		if err := s.nLink.LinkSetHardwareAddr(ctx, iface, in.BridgePort.Spec.MacAddress); err != nil {
			fmt.Printf("Failed to set MAC on link: %v", err)
			return err
		}
	}
	// Example: ip link set eth2 master br-tenant
	if err := s.nLink.LinkSetMaster(ctx, iface, bridge); err != nil {
		fmt.Printf("Failed to add iface to bridge: %v", err)
		return err
	}
	// add port to specified logical bridges
	for _, bridgeRefName := range in.BridgePort.Spec.LogicalBridges {
		fmt.Printf("add iface to logical bridge %s", bridgeRefName)
		// get object from DB
		bridgeObject := new(pb.LogicalBridge)
		ok, err := s.store.Get(bridgeRefName, bridgeObject)
		if err != nil {
			fmt.Printf("Failed to interact with store: %v", err)
			return err
		}
		if !ok {
			err := status.Errorf(codes.NotFound, "unable to find key %s", bridgeRefName)
			return err
		}
		vid := uint16(bridgeObject.Spec.VlanId)
		switch in.BridgePort.Spec.Ptype {
		case pb.BridgePortType_BRIDGE_PORT_TYPE_ACCESS:
			// Example: bridge vlan add dev eth2 vid 20 pvid untagged
			if err := s.nLink.BridgeVlanAdd(ctx, iface, vid, true, true, false, false); err != nil {
				fmt.Printf("Failed to add vlan to bridge: %v", err)
				return err
			}
		case pb.BridgePortType_BRIDGE_PORT_TYPE_TRUNK:
			// Example: bridge vlan add dev eth2 vid 20
			if err := s.nLink.BridgeVlanAdd(ctx, iface, vid, false, false, false, false); err != nil {
				fmt.Printf("Failed to add vlan to bridge: %v", err)
				return err
			}
		default:
			msg := fmt.Sprintf("Only ACCESS or TRUNK supported and not (%d)", in.BridgePort.Spec.Ptype)
			return status.Errorf(codes.InvalidArgument, msg)
		}
	}
	// Example: ip link set eth2 up
	if err := s.nLink.LinkSetUp(ctx, iface); err != nil {
		fmt.Printf("Failed to up iface link: %v", err)
		return err
	}
	return nil
}

func (s *Server) netlinkDeleteBridgePort(ctx context.Context, iface *pb.BridgePort) error {
	resourceID := path.Base(iface.Name)
	// use netlink to find interface
	dummy, err := s.nLink.LinkByName(ctx, resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		return err
	}
	// bring link down
	if err := s.nLink.LinkSetDown(ctx, dummy); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return err
	}
	// delete bridge vlan
	for _, bridgeRefName := range iface.Spec.LogicalBridges {
		// get object from DB
		bridgeObject := new(pb.LogicalBridge)
		ok, err := s.store.Get(bridgeRefName, bridgeObject)
		if err != nil {
			fmt.Printf("Failed to interact with store: %v", err)
			return err
		}
		if !ok {
			err := status.Errorf(codes.NotFound, "unable to find key %s", bridgeRefName)
			return err
		}
		vid := uint16(bridgeObject.Spec.VlanId)
		if err := s.nLink.BridgeVlanDel(ctx, dummy, vid, true, true, false, false); err != nil {
			fmt.Printf("Failed to delete vlan to bridge: %v", err)
			return err
		}
	}
	// use netlink to delete dummy interface
	if err := s.nLink.LinkDel(ctx, dummy); err != nil {
		fmt.Printf("Failed to delete link: %v", err)
		return err
	}
	return nil
}
