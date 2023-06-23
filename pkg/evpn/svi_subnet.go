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

	pb "github.com/opiproject/opi-api/network/cloud/v1alpha1/gen/go"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/resourceid"
	"go.einride.tech/aip/resourcename"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// CreateSubnet executes the creation of the SVI/subnet
func (s *Server) CreateSubnet(_ context.Context, in *pb.CreateSubnetRequest) (*pb.Subnet, error) {
	log.Printf("CreateSubnet: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.SubnetId != "" {
		err := resourceid.ValidateUserSettable(in.SubnetId)
		if err != nil {
			log.Printf("error: %v", err)
			return nil, err
		}
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.SubnetId, in.Subnet.Name)
		resourceID = in.SubnetId
	}
	in.Subnet.Name = fmt.Sprintf("//network.opiproject.org/subnets/%s", resourceID)
	// idempotent API when called with same key, should return same object
	snet, ok := s.Subnets[in.Subnet.Name]
	if ok {
		log.Printf("Already existing Subnet with id %v", in.Subnet.Name)
		return snet, nil
	}
	// not found, so create a new one
	bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: resourceID}}
	if err := netlink.LinkAdd(bridge); err != nil {
		fmt.Printf("Failed to create link: %v", err)
		return nil, err
	}
	// set MAC
	mac := in.Subnet.Spec.VirtualRouterMac
	if len(mac) > 0 {
		if err := netlink.LinkSetHardwareAddr(bridge, mac); err != nil {
			fmt.Printf("Failed to set MAC on link: %v", err)
			return nil, err
		}
	}
	// set IPv4
	if in.Subnet.Spec.V4Prefix.Addr > 0 && in.Subnet.Spec.V4Prefix.Len > 0 {
		myip := make(net.IP, 4)
		binary.BigEndian.PutUint32(myip, in.Subnet.Spec.V4Prefix.Addr)
		addr := &netlink.Addr{IPNet: &net.IPNet{IP: myip, Mask: net.CIDRMask(int(in.Subnet.Spec.V4Prefix.Len), 32)}}
		if err := netlink.AddrAdd(bridge, addr); err != nil {
			fmt.Printf("Failed to set IP on link: %v", err)
			return nil, err
		}
	}
	// Validate that a VRF/VPC resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Subnet.Spec.VpcNameRef); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// now get VRF/VPC to plug this bridge into
	vpc, ok := s.Vpcs[in.Subnet.Spec.VpcNameRef]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Subnet.Spec.VpcNameRef)
		log.Printf("error: %v", err)
		return nil, err
	}
	vrf, err := netlink.LinkByName(path.Base(vpc.Name))
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", vpc.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	if err := netlink.LinkSetMaster(bridge, vrf); err != nil {
		fmt.Printf("Failed to add bridge to vrf: %v", err)
		return nil, err
	}
	// set UP
	if err := netlink.LinkSetUp(bridge); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	// TODO: replace cloud -> evpn
	response := proto.Clone(in.Subnet).(*pb.Subnet)
	response.Status = &pb.SubnetStatus{HwIndex: 8, VnicCount: 88}
	s.Subnets[in.Subnet.Name] = response
	log.Printf("CreateSubnet: Sending to client: %v", response)
	return response, nil
}

// DeleteSubnet deletes a SVI/subnet
func (s *Server) DeleteSubnet(_ context.Context, in *pb.DeleteSubnetRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteSubnet: Received from client: %v", in)
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
	obj, ok := s.Subnets[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(obj.Name)
	// use netlink to find VRF/VPC
	vrf, err := netlink.LinkByName(resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		log.Printf("error: %v", err)
		return nil, err
	}
	// bring link down
	if err := netlink.LinkSetDown(vrf); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	// use netlink to delete VRF/VPC
	if err := netlink.LinkDel(vrf); err != nil {
		fmt.Printf("Failed to delete link: %v", err)
		return nil, err
	}
	// remove from the Database
	delete(s.Subnets, obj.Name)
	return &emptypb.Empty{}, nil
}

// UpdateSubnet updates an SVI/subnet
func (s *Server) UpdateSubnet(_ context.Context, in *pb.UpdateSubnetRequest) (*pb.Subnet, error) {
	log.Printf("UpdateSubnet: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Subnet.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	volume, ok := s.Subnets[in.Subnet.Name]
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Subnet.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(volume.Name)
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.Subnet); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	log.Printf("TODO: use resourceID=%v", resourceID)
	return nil, status.Errorf(codes.Unimplemented, "UpdateSubnet method is not implemented")
}

// GetSubnet gets an SVI/subnet
func (s *Server) GetSubnet(_ context.Context, in *pb.GetSubnetRequest) (*pb.Subnet, error) {
	log.Printf("GetSubnet: Received from client: %v", in)
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
	snet, ok := s.Subnets[in.Name]
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
	return &pb.Subnet{Name: in.Name, Spec: &pb.SubnetSpec{Ipv4VirtualRouterIp: snet.Spec.Ipv4VirtualRouterIp}, Status: &pb.SubnetStatus{HwIndex: 77}}, nil
}
