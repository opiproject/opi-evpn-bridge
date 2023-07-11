// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"fmt"
	"log"
	"path"
	"strconv"

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

// CreateVpc executes the creation of the VRF/VPC
func (s *Server) CreateVpc(_ context.Context, in *pb.CreateVpcRequest) (*pb.Vpc, error) {
	log.Printf("CreateVpc: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.VpcId != "" {
		err := resourceid.ValidateUserSettable(in.VpcId)
		if err != nil {
			log.Printf("error: %v", err)
			return nil, err
		}
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.VpcId, in.Vpc.Name)
		resourceID = in.VpcId
	}
	in.Vpc.Name = fmt.Sprintf("//network.opiproject.org/vpcs/%s", resourceID)
	// idempotent API when called with same key, should return same object
	obj, ok := s.Vpcs[in.Vpc.Name]
	if ok {
		log.Printf("Already existing Vpc with id %v", in.Vpc.Name)
		return obj, nil
	}
	// TODO: search DB for tables by this reference instead of reinterpreting this as integer
	table, err := strconv.Atoi(in.Vpc.Spec.V4RouteTableNameRef)
	if err != nil {
		err := status.Error(codes.InvalidArgument, "could not convert V4RouteTableNameRef to integer")
		log.Printf("error: %v", err)
		return nil, err
	}
	// not found, so create a new one
	vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: resourceID}, Table: uint32(table)}
	if err := netlink.LinkAdd(vrf); err != nil {
		fmt.Printf("Failed to create link: %v", err)
		return nil, err
	}
	if err := netlink.LinkSetUp(vrf); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	// TODO: replace cloud -> evpn
	response := proto.Clone(in.Vpc).(*pb.Vpc)
	response.Status = &pb.VpcStatus{SubnetCount: 4}
	s.Vpcs[in.Vpc.Name] = response
	log.Printf("CreateVpc: Sending to client: %v", response)
	return response, nil
}

// DeleteVpc deletes a VRF/VPC
func (s *Server) DeleteVpc(_ context.Context, in *pb.DeleteVpcRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteVpc: Received from client: %v", in)
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
	obj, ok := s.Vpcs[in.Name]
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
	delete(s.Vpcs, obj.Name)
	return &emptypb.Empty{}, nil
}

// UpdateVpc updates an VRF/VPC
func (s *Server) UpdateVpc(_ context.Context, in *pb.UpdateVpcRequest) (*pb.Vpc, error) {
	log.Printf("UpdateVpc: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Vpc.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	volume, ok := s.Vpcs[in.Vpc.Name]
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Vpc.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.Vpc); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(volume.Name)
	_, err := netlink.LinkByName(resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		log.Printf("error: %v", err)
		return nil, err
	}
	// base := iface.Attrs()
	// iface.MTU = 1500 // TODO: remove this, just an example
	// if err := netlink.LinkModify(iface); err != nil {
	// 	fmt.Printf("Failed to update link: %v", err)
	// 	return nil, err
	// }
	log.Printf("TODO: use resourceID=%v", resourceID)
	return nil, status.Errorf(codes.Unimplemented, "UpdateVpc method is not implemented")
}

// GetVpc gets an VRF/VPC
func (s *Server) GetVpc(_ context.Context, in *pb.GetVpcRequest) (*pb.Vpc, error) {
	log.Printf("GetVpc: Received from client: %v", in)
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
	obj, ok := s.Vpcs[in.Name]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(obj.Name)
	_, err := netlink.LinkByName(resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		log.Printf("error: %v", err)
		return nil, err
	}
	// TODO
	return &pb.Vpc{Name: in.Name, Spec: &pb.VpcSpec{V4RouteTableNameRef: obj.Spec.V4RouteTableNameRef, Tos: 11}, Status: &pb.VpcStatus{SubnetCount: 77}}, nil
}
