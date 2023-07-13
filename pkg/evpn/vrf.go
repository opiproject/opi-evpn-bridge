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

// CreateVrf executes the creation of the VRF
func (s *Server) CreateVrf(_ context.Context, in *pb.CreateVrfRequest) (*pb.Vrf, error) {
	log.Printf("CreateVrf: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.VrfId != "" {
		err := resourceid.ValidateUserSettable(in.VrfId)
		if err != nil {
			log.Printf("error: %v", err)
			return nil, err
		}
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.VrfId, in.Vrf.Name)
		resourceID = in.VrfId
	}
	in.Vrf.Name = resourceIDToFullName("vrfs", resourceID)
	// idempotent API when called with same key, should return same object
	obj, ok := s.Vrfs[in.Vrf.Name]
	if ok {
		log.Printf("Already existing Vrf with id %v", in.Vrf.Name)
		return obj, nil
	}
	// not found, so create a new one
	vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: resourceID}, Table: in.Vrf.Spec.Vni}
	if err := netlink.LinkAdd(vrf); err != nil {
		fmt.Printf("Failed to create link: %v", err)
		return nil, err
	}
	if err := netlink.LinkSetUp(vrf); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	response := proto.Clone(in.Vrf).(*pb.Vrf)
	response.Status = &pb.VrfStatus{LocalAs: 4}
	s.Vrfs[in.Vrf.Name] = response
	log.Printf("CreateVrf: Sending to client: %v", response)
	return response, nil
}

// DeleteVrf deletes a VRF
func (s *Server) DeleteVrf(_ context.Context, in *pb.DeleteVrfRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteVrf: Received from client: %v", in)
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
	obj, ok := s.Vrfs[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(obj.Name)
	// use netlink to find VRF
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
	// use netlink to delete VRF
	if err := netlink.LinkDel(vrf); err != nil {
		fmt.Printf("Failed to delete link: %v", err)
		return nil, err
	}
	// remove from the Database
	delete(s.Vrfs, obj.Name)
	return &emptypb.Empty{}, nil
}

// UpdateVrf updates an VRF
func (s *Server) UpdateVrf(_ context.Context, in *pb.UpdateVrfRequest) (*pb.Vrf, error) {
	log.Printf("UpdateVrf: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Vrf.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	volume, ok := s.Vrfs[in.Vrf.Name]
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Vrf.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.Vrf); err != nil {
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
	response := proto.Clone(in.Vrf).(*pb.Vrf)
	response.Status = &pb.VrfStatus{LocalAs: 4}
	s.Vrfs[in.Vrf.Name] = response
	log.Printf("UpdateVrf: Sending to client: %v", response)
	return response, nil
}

// GetVrf gets an VRF
func (s *Server) GetVrf(_ context.Context, in *pb.GetVrfRequest) (*pb.Vrf, error) {
	log.Printf("GetVrf: Received from client: %v", in)
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
	obj, ok := s.Vrfs[in.Name]
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
	return &pb.Vrf{Name: in.Name, Spec: &pb.VrfSpec{Vni: obj.Spec.Vni}, Status: &pb.VrfStatus{LocalAs: 77}}, nil
}
