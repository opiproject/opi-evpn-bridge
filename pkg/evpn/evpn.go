// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.

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
	pc "github.com/opiproject/opi-api/network/opinetcommon/v1alpha1/gen/go"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/resourceid"
	"go.einride.tech/aip/resourcename"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Server represents the Server object
type Server struct {
	pb.UnimplementedCloudInfraServiceServer
	Subnets    map[string]*pb.Subnet
	Interfaces map[string]*pb.Interface
	Vpcs       map[string]*pb.Vpc
}

// NewServer creates initialized instance of EVPN server
func NewServer() *Server {
	return &Server{
		Subnets:    make(map[string]*pb.Subnet),
		Interfaces: make(map[string]*pb.Interface),
		Vpcs:       make(map[string]*pb.Vpc),
	}
}

// CreateInterface executes the creation of the interface
func (s *Server) CreateInterface(_ context.Context, in *pb.CreateInterfaceRequest) (*pb.Interface, error) {
	log.Printf("CreateInterface: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.InterfaceId != "" {
		err := resourceid.ValidateUserSettable(in.InterfaceId)
		if err != nil {
			log.Printf("error: %v", err)
			return nil, err
		}
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.InterfaceId, in.Interface.Name)
		resourceID = in.InterfaceId
	}
	in.Interface.Name = fmt.Sprintf("//network.opiproject.org/interfaces/%s", resourceID)
	// idempotent API when called with same key, should return same object
	iface, ok := s.Interfaces[in.Interface.Name]
	if ok {
		log.Printf("Already existing Interface with id %v", in.Interface.Name)
		return iface, nil
	}
	// create dummy interface
	dummy := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: resourceID}}
	if err := netlink.LinkAdd(dummy); err != nil {
		fmt.Printf("Failed to create link: %v", err)
		return nil, err
	}
	switch u := in.Interface.Spec.Ifinfo.(type) {
	case *pb.InterfaceSpec_ControlIfSpec:
		// set MAC
		mac := u.ControlIfSpec.MacAddress
		if len(mac) > 0 {
			if err := netlink.LinkSetHardwareAddr(dummy, mac); err != nil {
				fmt.Printf("Failed to set MAC on link: %v", err)
				return nil, err
			}
		}
		// set IPv4
		prefix := u.ControlIfSpec.Prefix
		switch a := prefix.Addr.V4OrV6.(type) {
		case *pc.IPAddress_V4Addr:
			if prefix.Addr.Af == pc.IpAf_IP_AF_INET && prefix.Len > 0 && a.V4Addr > 0 {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, a.V4Addr)
				addr := &netlink.Addr{IPNet: &net.IPNet{IP: myip, Mask: net.CIDRMask(int(prefix.Len), 32)}}
				if err := netlink.AddrAdd(dummy, addr); err != nil {
					fmt.Printf("Failed to set IP on link: %v", err)
					return nil, err
				}
			}
		default:
			fmt.Println("No matching operations")
		}
	default:
		fmt.Println("No matching operations")
	}
	if err := netlink.LinkSetUp(dummy); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	// TODO: do we also need to add this link to the bridge here ? search s.Subnets ?
	response := proto.Clone(in.Interface).(*pb.Interface)
	response.Status = &pb.InterfaceStatus{IfIndex: 8, OperStatus: pb.IfStatus_IF_STATUS_UP}
	s.Interfaces[in.Interface.Name] = response
	log.Printf("CreateInterface: Sending to client: %v", response)
	return response, nil
}

// DeleteInterface deletes an interface
func (s *Server) DeleteInterface(_ context.Context, in *pb.DeleteInterfaceRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteInterface: Received from client: %v", in)
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
	iface, ok := s.Interfaces[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(iface.Name)
	// use netlink to find interface
	dummy, err := netlink.LinkByName(resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		log.Printf("error: %v", err)
		return nil, err
	}
	// bring link down
	if err := netlink.LinkSetDown(dummy); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	// use netlink to delete dummy interface
	if err := netlink.LinkDel(dummy); err != nil {
		fmt.Printf("Failed to delete link: %v", err)
		return nil, err
	}
	// remove from the Database
	delete(s.Interfaces, iface.Name)
	return &emptypb.Empty{}, nil
}

// UpdateInterface updates an Nvme Subsystem
func (s *Server) UpdateInterface(_ context.Context, in *pb.UpdateInterfaceRequest) (*pb.Interface, error) {
	log.Printf("UpdateInterface: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Interface.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	volume, ok := s.Interfaces[in.Interface.Name]
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Interface.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(volume.Name)
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.Interface); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	log.Printf("TODO: use resourceID=%v", resourceID)
	// TODO: modify dummy interface
	// dummy := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: resourceID}}
	// if err := netlink.LinkModify(dummy); err != nil {
	// 	fmt.Printf("Failed to delete link: %v", err)
	// 	return nil, err
	// }
	return nil, status.Errorf(codes.Unimplemented, "UpdateInterface method is not implemented")
}

// GetInterface gets an Interface
func (s *Server) GetInterface(_ context.Context, in *pb.GetInterfaceRequest) (*pb.Interface, error) {
	log.Printf("GetInterface: Received from client: %v", in)
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
	snet, ok := s.Interfaces[in.Name]
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
	return &pb.Interface{Name: in.Name, Spec: &pb.InterfaceSpec{Ifid: snet.Spec.Ifid}, Status: &pb.InterfaceStatus{IfIndex: 55}}, nil
}
