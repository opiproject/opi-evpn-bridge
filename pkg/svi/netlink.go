// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package svi is the main package of the application
package svi

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"path"

	"github.com/vishvananda/netlink"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) netlinkCreateSvi(ctx context.Context, in *pb.CreateSviRequest, bridgeObject *pb.LogicalBridge, vrf *pb.Vrf) error {
	// use netlink to find br-tenant
	bridge, err := s.nLink.LinkByName(ctx, tenantbridgeName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", tenantbridgeName)
		return err
	}
	vid := uint16(bridgeObject.Spec.VlanId)
	// Example: bridge vlan add dev br-tenant vid <vlan-id> self
	if err := s.nLink.BridgeVlanAdd(ctx, bridge, vid, false, false, true, false); err != nil {
		fmt.Printf("Failed to add vlan to bridge: %v", err)
		return err
	}
	// Example: ip link add link br-tenant name <link_svi> type vlan id <vlan-id>
	vlanName := fmt.Sprintf("vlan%d", vid)
	vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
	log.Printf("Creating VLAN %v", vlandev)
	if err := s.nLink.LinkAdd(ctx, vlandev); err != nil {
		fmt.Printf("Failed to create vlan link: %v", err)
		return err
	}
	// Example: ip link set <link_svi> addr aa:bb:cc:00:00:41
	if len(in.Svi.Spec.MacAddress) > 0 {
		if err := s.nLink.LinkSetHardwareAddr(ctx, vlandev, in.Svi.Spec.MacAddress); err != nil {
			fmt.Printf("Failed to set MAC on link: %v", err)
			return err
		}
	}
	// Example: ip address add <svi-ip-with prefixlength> dev <link_svi>
	for _, gwip := range in.Svi.Spec.GwIpPrefix {
		fmt.Printf("Assign the GW IP address %v to the SVI interface %v", gwip, vlandev)
		myip := make(net.IP, 4)
		binary.BigEndian.PutUint32(myip, gwip.Addr.GetV4Addr())
		addr := &netlink.Addr{IPNet: &net.IPNet{IP: myip, Mask: net.CIDRMask(int(gwip.Len), 32)}}
		if err := s.nLink.AddrAdd(ctx, vlandev, addr); err != nil {
			fmt.Printf("Failed to set IP on link: %v", err)
			return err
		}
	}
	// get net device by name
	vrfName := path.Base(vrf.Name)
	vrfdev, err := s.nLink.LinkByName(ctx, vrfName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", vrf.Name)
		return err
	}
	// Example: ip link set <link_svi> master <vrf-name> up
	if err := s.nLink.LinkSetMaster(ctx, vlandev, vrfdev); err != nil {
		fmt.Printf("Failed to add vlandev to vrf: %v", err)
		return err
	}
	// Example: ip link set <link_svi> up
	if err := s.nLink.LinkSetUp(ctx, vlandev); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return err
	}
	return nil
}

func (s *Server) netlinkDeleteSvi(ctx context.Context, _ *pb.DeleteSviRequest, bridgeObject *pb.LogicalBridge, _ *pb.Vrf) error {
	// use netlink to find br-tenant
	bridge, err := s.nLink.LinkByName(ctx, tenantbridgeName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", tenantbridgeName)
		return err
	}
	vid := uint16(bridgeObject.Spec.VlanId)
	// Example: bridge vlan del dev br-tenant vid <vlan-id> self
	if err := s.nLink.BridgeVlanDel(ctx, bridge, vid, false, false, true, false); err != nil {
		fmt.Printf("Failed to del vlan to bridge: %v", err)
		return err
	}
	vlanName := fmt.Sprintf("vlan%d", vid)
	vlandev, err := s.nLink.LinkByName(ctx, vlanName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", vlanName)
		return err
	}
	log.Printf("Deleting VLAN %v", vlandev)
	// bring link down
	if err := s.nLink.LinkSetDown(ctx, vlandev); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return err
	}
	// use netlink to delete vlan
	if err := s.nLink.LinkDel(ctx, vlandev); err != nil {
		fmt.Printf("Failed to delete link: %v", err)
		return err
	}
	return nil
}
