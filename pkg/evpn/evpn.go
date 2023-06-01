// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"fmt"
	"log"

	"github.com/milosgajdos/tenus"
	pb "github.com/opiproject/opi-api/network/cloud/v1alpha1/gen/go"
	"github.com/ulule/deepcopier"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Server represents the Server object
type Server struct {
	pb.UnimplementedCloudInfraServiceServer
	Subnets    map[string]*pb.Subnet
	Interfaces map[string]*pb.Interface
}

// CreateSubnet executes the creation of the subnet
func (s *Server) CreateSubnet(_ context.Context, in *pb.CreateSubnetRequest) (*pb.Subnet, error) {
	log.Printf("CreateSubnet: Received from client: %v", in)
	// idempotent API when called with same key, should return same object
	snet, ok := s.Subnets[in.Subnet.Name]
	if ok {
		log.Printf("Already existing Subnet with id %v", in.Subnet.Name)
		return snet, nil
	}
	// not found, so create a new one

	// Create a new network bridge
	// It is equivalent of running: ip link add name ${ifcName} type bridge
	br, err := tenus.NewBridgeWithName("mybridge")
	if err != nil {
		log.Fatal(err)
	}

	// Bring the bridge up
	if err = br.SetLinkUp(); err != nil {
		fmt.Println(err)
	}

	// Create a dummy link
	dl, err := tenus.NewLink("mydummylink")
	if err != nil {
		log.Fatal(err)
	}

	// Add the dummy link into bridge
	if err = br.AddSlaveIfc(dl.NetInterface()); err != nil {
		log.Fatal(err)
	}

	// Bring the dummy link up
	if err = dl.SetLinkUp(); err != nil {
		fmt.Println(err)
	}
	// TODO: replace cloud -> evpn
	s.Subnets[in.Subnet.Name] = in.Subnet
	response := &pb.Subnet{}
	err = deepcopier.Copy(in.Subnet).To(response)
	if err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	response.Status = &pb.SubnetStatus{HwIndex: 8}
	return response, nil
}

// DeleteSubnet deletes a subnet
func (s *Server) DeleteSubnet(_ context.Context, in *pb.DeleteSubnetRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteSubnet: Received from client: %v", in)
	snet, ok := s.Subnets[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// Get an existing network bridge
	br, err := tenus.BridgeFromName("mybridge")
	if err != nil {
		log.Fatal(err)
	}
	// Bring the bridge down
	if err = br.SetLinkDown(); err != nil {
		fmt.Println(err)
	}
	// Delete link
	// $ sudo ip link delete br0 type bridge
	if err = tenus.DeleteLink("mybridge"); err != nil {
		log.Fatal(err)
	}

	delete(s.Subnets, snet.Name)
	return &emptypb.Empty{}, nil
}

// CreateInterface executes the creation of the interface
func (s *Server) CreateInterface(_ context.Context, in *pb.CreateInterfaceRequest) (*pb.Interface, error) {
	log.Printf("CreateInterface: Received from client: %v", in)
	// idempotent API when called with same key, should return same object
	iface, ok := s.Interfaces[in.Interface.Name]
	if ok {
		log.Printf("Already existing Interface with id %v", in.Interface.Name)
		return iface, nil
	}
	// not found, so create a new one
	snet, ok := s.Subnets[in.Interface.Name]
	if !ok {
		// TODO: change Spec.Id.Value to bridge reference instead
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Interface.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// Get an existing network bridge
	br, err := tenus.BridgeFromName(snet.Name)
	if err != nil {
		log.Fatal(err)
	}
	// Create a dummy link
	dl, err := tenus.NewLink("mydummylink")
	if err != nil {
		log.Fatal(err)
	}
	// Add the dummy link into bridge
	if err = br.AddSlaveIfc(dl.NetInterface()); err != nil {
		log.Fatal(err)
	}

	// Bring the dummy link up
	if err = dl.SetLinkUp(); err != nil {
		fmt.Println(err)
	}
	// TODO: replace cloud -> evpn
	s.Interfaces[in.Interface.Name] = in.Interface
	response := &pb.Interface{}
	err = deepcopier.Copy(in.Interface).To(response)
	if err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	response.Status = &pb.InterfaceStatus{IfIndex: 8}
	return response, nil
}

// DeleteInterface deletes an interface
func (s *Server) DeleteInterface(_ context.Context, in *pb.DeleteInterfaceRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteInterface: Received from client: %v", in)
	iface, ok := s.Interfaces[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}

	// Delete link
	// $ sudo ip link delete br0 type bridge
	if err := tenus.DeleteLink(iface.Name); err != nil {
		log.Fatal(err)
	}

	delete(s.Interfaces, iface.Name)
	return &emptypb.Empty{}, nil
}
