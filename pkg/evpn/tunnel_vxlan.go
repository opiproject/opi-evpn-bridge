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

// CreateTunnel executes the creation of the VXLAN
func (s *Server) CreateTunnel(_ context.Context, in *pb.CreateTunnelRequest) (*pb.Tunnel, error) {
	log.Printf("CreateTunnel: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// validate input parameters since they are not manadatory in protobuf yet
	if in.Tunnel.Spec.LocalIp == nil || in.Tunnel.Spec.Encap == nil {
		msg := fmt.Sprintf("Missing LocalIp or Encap manadatory fields for %s", in.TunnelId)
		log.Print(msg)
		return nil, status.Errorf(codes.InvalidArgument, msg)
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.TunnelId != "" {
		err := resourceid.ValidateUserSettable(in.TunnelId)
		if err != nil {
			log.Printf("error: %v", err)
			return nil, err
		}
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.TunnelId, in.Tunnel.Name)
		resourceID = in.TunnelId
	}
	in.Tunnel.Name = resourceIDToFullName("tunnels", resourceID)
	// idempotent API when called with same key, should return same object
	obj, ok := s.Tunnels[in.Tunnel.Name]
	if ok {
		log.Printf("Already existing Tunnel with id %v", in.Tunnel.Name)
		return obj, nil
	}
	// not found, so create a new one
	myip := make(net.IP, 4)
	binary.BigEndian.PutUint32(myip, in.Tunnel.Spec.LocalIp.GetV4Addr())
	vxlanid := in.Tunnel.Spec.Encap.Value.GetVnid()
	// TODO: take Port from proto instead of hard-coded
	vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: resourceID}, VxlanId: int(vxlanid), Port: 4789, Learning: false, SrcAddr: myip}
	if err := netlink.LinkAdd(vxlan); err != nil {
		fmt.Printf("Failed to create link: %v", err)
		return nil, err
	}
	// TODO: gap... instead of VPC we are looking for Subnet/Bridge here...
	if in.Tunnel.Spec.VpcNameRef != "" {
		// Validate that a Subnet/Bridge resource name conforms to the restrictions outlined in AIP-122.
		if err := resourcename.Validate(in.Tunnel.Spec.VpcNameRef); err != nil {
			log.Printf("error: %v", err)
			return nil, err
		}
		// now get Subnet/Bridge to plug this vxlan into
		bridge, ok := s.Subnets[in.Tunnel.Spec.VpcNameRef]
		if !ok {
			err := status.Errorf(codes.NotFound, "unable to find key %s", in.Tunnel.Spec.VpcNameRef)
			log.Printf("error: %v", err)
			return nil, err
		}
		brdev, err := netlink.LinkByName(path.Base(bridge.Name))
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", bridge.Name)
			log.Printf("error: %v", err)
			return nil, err
		}
		if err := netlink.LinkSetMaster(vxlan, brdev); err != nil {
			fmt.Printf("Failed to add vxlan to bridge: %v", err)
			return nil, err
		}
	}
	if err := netlink.LinkSetUp(vxlan); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	// TODO: replace cloud -> evpn
	response := proto.Clone(in.Tunnel).(*pb.Tunnel)
	response.Status = &pb.TunnelStatus{VnicCount: 4}
	s.Tunnels[in.Tunnel.Name] = response
	log.Printf("CreateTunnel: Sending to client: %v", response)
	return response, nil
}

// DeleteTunnel deletes a Subnet/Bridge
func (s *Server) DeleteTunnel(_ context.Context, in *pb.DeleteTunnelRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteTunnel: Received from client: %v", in)
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
	obj, ok := s.Tunnels[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(obj.Name)
	// use netlink to find Subnet/Bridge
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
	// use netlink to delete Subnet/Bridge
	if err := netlink.LinkDel(vrf); err != nil {
		fmt.Printf("Failed to delete link: %v", err)
		return nil, err
	}
	// remove from the Database
	delete(s.Tunnels, obj.Name)
	return &emptypb.Empty{}, nil
}

// UpdateTunnel updates an Subnet/Bridge
func (s *Server) UpdateTunnel(_ context.Context, in *pb.UpdateTunnelRequest) (*pb.Tunnel, error) {
	log.Printf("UpdateTunnel: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Tunnel.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	volume, ok := s.Tunnels[in.Tunnel.Name]
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Tunnel.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.Tunnel); err != nil {
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
	response := proto.Clone(in.Tunnel).(*pb.Tunnel)
	response.Status = &pb.TunnelStatus{VnicCount: 4}
	s.Tunnels[in.Tunnel.Name] = response
	log.Printf("UpdateTunnel: Sending to client: %v", response)
	return response, nil
}

// GetTunnel gets an Subnet/Bridge
func (s *Server) GetTunnel(_ context.Context, in *pb.GetTunnelRequest) (*pb.Tunnel, error) {
	log.Printf("GetTunnel: Received from client: %v", in)
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
	obj, ok := s.Tunnels[in.Name]
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
	return &pb.Tunnel{Name: in.Name, Spec: &pb.TunnelSpec{Tos: 11}, Status: &pb.TunnelStatus{VnicCount: 77}}, nil
}
