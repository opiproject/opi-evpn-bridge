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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) netlinkCreateVrf(ctx context.Context, in *pb.CreateVrfRequest, tableID uint32, mac []byte) error {
	vrfName := path.Base(in.Vrf.Name)
	// Example: ip link add blue type vrf table 1000
	vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: vrfName}, Table: tableID}
	log.Printf("Creating VRF %v", vrf)
	if err := s.nLink.LinkAdd(ctx, vrf); err != nil {
		fmt.Printf("Failed to create VRF link: %v", err)
		return err
	}
	// Example: ip link set blue up
	if err := s.nLink.LinkSetUp(ctx, vrf); err != nil {
		fmt.Printf("Failed to up VRF link: %v", err)
		return err
	}
	// Example: ip address add <vrf-loopback> dev <vrf-name>
	if in.Vrf.Spec.LoopbackIpPrefix != nil && in.Vrf.Spec.LoopbackIpPrefix.Addr != nil && in.Vrf.Spec.LoopbackIpPrefix.Len > 0 {
		myip := make(net.IP, 4)
		binary.BigEndian.PutUint32(myip, in.Vrf.Spec.LoopbackIpPrefix.Addr.GetV4Addr())
		addr := &netlink.Addr{IPNet: &net.IPNet{IP: myip, Mask: net.CIDRMask(int(in.Vrf.Spec.LoopbackIpPrefix.Len), 32)}}
		if err := s.nLink.AddrAdd(ctx, vrf, addr); err != nil {
			fmt.Printf("Failed to set IP on VRF link: %v", err)
			return err
		}
	}
	// TODO: Add low-prio default route. Otherwise a miss leads to lookup in the next higher table
	// Example: ip route add throw default table <routing-table-number> proto evpn-gw-br metric 9999

	// create bridge and vxlan only if VNI value is not empty
	if in.Vrf.Spec.Vni != nil {
		// Example: ip link add br100 type bridge
		bridgeName := fmt.Sprintf("br%d", *in.Vrf.Spec.Vni)
		bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
		log.Printf("Creating Linux Bridge %v", bridge)
		if err := s.nLink.LinkAdd(ctx, bridge); err != nil {
			fmt.Printf("Failed to create Bridge link: %v", err)
			return err
		}
		// Example: ip link set br100 master blue addrgenmode none
		if err := s.nLink.LinkSetMaster(ctx, bridge, vrf); err != nil {
			fmt.Printf("Failed to add Bridge to VRF: %v", err)
			return err
		}
		// Example: ip link set br100 addr aa:bb:cc:00:00:02
		if err := s.nLink.LinkSetHardwareAddr(ctx, bridge, mac); err != nil {
			fmt.Printf("Failed to set MAC on Bridge link: %v", err)
			return err
		}
		// Example: ip link set br100 up
		if err := s.nLink.LinkSetUp(ctx, bridge); err != nil {
			fmt.Printf("Failed to up Bridge link: %v", err)
			return err
		}
		// Example: ip link add vni100 type vxlan local 10.0.0.4 dstport 4789 id 100 nolearning
		vxlanName := fmt.Sprintf("vni%d", *in.Vrf.Spec.Vni)
		myip := make(net.IP, 4)
		binary.BigEndian.PutUint32(myip, in.Vrf.Spec.VtepIpPrefix.Addr.GetV4Addr())
		// TODO: take Port from proto instead of hard-coded
		vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*in.Vrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
		log.Printf("Creating VXLAN %v", vxlan)
		if err := s.nLink.LinkAdd(ctx, vxlan); err != nil {
			fmt.Printf("Failed to create Vxlan link: %v", err)
			return err
		}
		// Example: ip link set vni100 master br100 addrgenmode none
		if err := s.nLink.LinkSetMaster(ctx, vxlan, bridge); err != nil {
			fmt.Printf("Failed to add Vxlan to bridge: %v", err)
			return err
		}
		// Example: ip link set vni100 up
		if err := s.nLink.LinkSetUp(ctx, vxlan); err != nil {
			fmt.Printf("Failed to up Vxlan link: %v", err)
			return err
		}
	}
	return nil
}

func (s *Server) netlinkDeleteVrf(ctx context.Context, obj *pb.Vrf) error {
	// delete bridge and vxlan only if VNI value is not empty
	if obj.Spec.Vni != nil {
		// use netlink to find VXLAN device
		vxlanName := fmt.Sprintf("vni%d", *obj.Spec.Vni)
		vxlandev, err := s.nLink.LinkByName(ctx, vxlanName)
		log.Printf("Deleting VXLAN %v", vxlandev)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", vxlanName)
			return err
		}
		// bring link down
		if err := s.nLink.LinkSetDown(ctx, vxlandev); err != nil {
			fmt.Printf("Failed to up link: %v", err)
			return err
		}
		// use netlink to delete VXLAN device
		if err := s.nLink.LinkDel(ctx, vxlandev); err != nil {
			fmt.Printf("Failed to delete link: %v", err)
			return err
		}
		// use netlink to find BRIDGE device
		bridgeName := fmt.Sprintf("br%d", *obj.Spec.Vni)
		bridgedev, err := s.nLink.LinkByName(ctx, bridgeName)
		log.Printf("Deleting BRIDGE %v", bridgedev)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", bridgeName)
			return err
		}
		// bring link down
		if err := s.nLink.LinkSetDown(ctx, bridgedev); err != nil {
			fmt.Printf("Failed to up link: %v", err)
			return err
		}
		// use netlink to delete BRIDGE device
		if err := s.nLink.LinkDel(ctx, bridgedev); err != nil {
			fmt.Printf("Failed to delete link: %v", err)
			return err
		}
	}
	vrfName := path.Base(obj.Name)
	// use netlink to find VRF
	vrf, err := s.nLink.LinkByName(ctx, vrfName)
	log.Printf("Deleting VRF %v", vrf)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", vrfName)
		return err
	}
	// bring link down
	if err := s.nLink.LinkSetDown(ctx, vrf); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return err
	}
	// use netlink to delete VRF
	if err := s.nLink.LinkDel(ctx, vrf); err != nil {
		fmt.Printf("Failed to delete link: %v", err)
		return err
	}
	return nil
}
