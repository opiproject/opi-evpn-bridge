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
	"sort"

	"github.com/google/uuid"
	// "github.com/vishvananda/netlink"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/resourceid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func sortBridgePorts(ports []*pb.BridgePort) {
	sort.Slice(ports, func(i int, j int) bool {
		return ports[i].Name < ports[j].Name
	})
}

// CreateBridgePort executes the creation of the port
func (s *Server) CreateBridgePort(ctx context.Context, in *pb.CreateBridgePortRequest) (*pb.BridgePort, error) {
	// check input correctness
	if err := s.validateCreateBridgePortRequest(in); err != nil {
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.BridgePortId != "" {
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.BridgePortId, in.BridgePort.Name)
		resourceID = in.BridgePortId
	}
	in.BridgePort.Name = resourceIDToFullName("ports", resourceID)
	// idempotent API when called with same key, should return same object
	obj, ok := s.Ports[in.BridgePort.Name]
	if ok {
		log.Printf("Already existing BridgePort with id %v", in.BridgePort.Name)
		return obj, nil
	}
	// not found, so create a new one
	bridge, err := s.nLink.LinkByName(ctx, tenantbridgeName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", tenantbridgeName)
		return nil, err
	}
	// get base interface (e.g.: eth2)
	iface, err := s.nLink.LinkByName(ctx, resourceID)
	// TODO: maybe we need to create a new iface here and not rely on existing one ?
	//		 iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: resourceID}}
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		return nil, err
	}
	// Example: ip link set eth2 addr aa:bb:cc:00:00:41
	if len(in.BridgePort.Spec.MacAddress) > 0 {
		if err := s.nLink.LinkSetHardwareAddr(ctx, iface, in.BridgePort.Spec.MacAddress); err != nil {
			fmt.Printf("Failed to set MAC on link: %v", err)
			return nil, err
		}
	}
	// Example: ip link set eth2 master br-tenant
	if err := s.nLink.LinkSetMaster(ctx, iface, bridge); err != nil {
		fmt.Printf("Failed to add iface to bridge: %v", err)
		return nil, err
	}
	// add port to specified logical bridges
	for _, bridgeRefName := range in.BridgePort.Spec.LogicalBridges {
		fmt.Printf("add iface to logical bridge %s", bridgeRefName)
		// get object from DB
		bridgeObject, ok := s.Bridges[bridgeRefName]
		if !ok {
			err := status.Errorf(codes.NotFound, "unable to find key %s", bridgeRefName)
			return nil, err
		}
		vid := uint16(bridgeObject.Spec.VlanId)
		switch in.BridgePort.Spec.Ptype {
		case pb.BridgePortType_ACCESS:
			// Example: bridge vlan add dev eth2 vid 20 pvid untagged
			if err := s.nLink.BridgeVlanAdd(ctx, iface, vid, true, true, false, false); err != nil {
				fmt.Printf("Failed to add vlan to bridge: %v", err)
				return nil, err
			}
		case pb.BridgePortType_TRUNK:
			// Example: bridge vlan add dev eth2 vid 20
			if err := s.nLink.BridgeVlanAdd(ctx, iface, vid, false, false, false, false); err != nil {
				fmt.Printf("Failed to add vlan to bridge: %v", err)
				return nil, err
			}
		default:
			msg := fmt.Sprintf("Only ACCESS or TRUNK supported and not (%d)", in.BridgePort.Spec.Ptype)
			return nil, status.Errorf(codes.InvalidArgument, msg)
		}
	}
	// Example: ip link set eth2 up
	if err := s.nLink.LinkSetUp(ctx, iface); err != nil {
		fmt.Printf("Failed to up iface link: %v", err)
		return nil, err
	}
	response := protoClone(in.BridgePort)
	response.Status = &pb.BridgePortStatus{OperStatus: pb.BPOperStatus_BP_OPER_STATUS_UP}
	s.Ports[in.BridgePort.Name] = response
	return response, nil
}

// DeleteBridgePort deletes a port
func (s *Server) DeleteBridgePort(ctx context.Context, in *pb.DeleteBridgePortRequest) (*emptypb.Empty, error) {
	// check input correctness
	if err := s.validateDeleteBridgePortRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	iface, ok := s.Ports[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return nil, err
	}
	resourceID := path.Base(iface.Name)
	// use netlink to find interface
	dummy, err := s.nLink.LinkByName(ctx, resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		return nil, err
	}
	// bring link down
	if err := s.nLink.LinkSetDown(ctx, dummy); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	// delete bridge vlan
	for _, bridgeRefName := range iface.Spec.LogicalBridges {
		// get object from DB
		bridgeObject, ok := s.Bridges[bridgeRefName]
		if !ok {
			err := status.Errorf(codes.NotFound, "unable to find key %s", bridgeRefName)
			return nil, err
		}
		vid := uint16(bridgeObject.Spec.VlanId)
		if err := s.nLink.BridgeVlanDel(ctx, dummy, vid, true, true, false, false); err != nil {
			fmt.Printf("Failed to delete vlan to bridge: %v", err)
			return nil, err
		}
	}
	// use netlink to delete dummy interface
	if err := s.nLink.LinkDel(ctx, dummy); err != nil {
		fmt.Printf("Failed to delete link: %v", err)
		return nil, err
	}
	// remove from the Database
	delete(s.Ports, iface.Name)
	return &emptypb.Empty{}, nil
}

// UpdateBridgePort updates an Nvme Subsystem
func (s *Server) UpdateBridgePort(ctx context.Context, in *pb.UpdateBridgePortRequest) (*pb.BridgePort, error) {
	// check input correctness
	if err := s.validateUpdateBridgePortRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	port, ok := s.Ports[in.BridgePort.Name]
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.BridgePort.Name)
		return nil, err
	}
	resourceID := path.Base(port.Name)
	iface, err := s.nLink.LinkByName(ctx, resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		return nil, err
	}
	// base := iface.Attrs()
	// iface.MTU = 1500 // TODO: remove this, just an example
	if err := s.nLink.LinkModify(ctx, iface); err != nil {
		fmt.Printf("Failed to update link: %v", err)
		return nil, err
	}
	response := protoClone(in.BridgePort)
	response.Status = &pb.BridgePortStatus{OperStatus: pb.BPOperStatus_BP_OPER_STATUS_UP}
	s.Ports[in.BridgePort.Name] = response
	return response, nil
}

// GetBridgePort gets an BridgePort
func (s *Server) GetBridgePort(ctx context.Context, in *pb.GetBridgePortRequest) (*pb.BridgePort, error) {
	// check input correctness
	if err := s.validateGetBridgePortRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	port, ok := s.Ports[in.Name]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return nil, err
	}
	resourceID := path.Base(port.Name)
	_, err := s.nLink.LinkByName(ctx, resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		return nil, err
	}
	// TODO
	return &pb.BridgePort{Name: in.Name, Spec: &pb.BridgePortSpec{MacAddress: port.Spec.MacAddress}, Status: &pb.BridgePortStatus{OperStatus: pb.BPOperStatus_BP_OPER_STATUS_UP}}, nil
}

// ListBridgePorts lists logical bridges
func (s *Server) ListBridgePorts(_ context.Context, in *pb.ListBridgePortsRequest) (*pb.ListBridgePortsResponse, error) {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return nil, err
	}
	// fetch pagination from the database, calculate size and offset
	size, offset, perr := extractPagination(in.PageSize, in.PageToken, s.Pagination)
	if perr != nil {
		return nil, perr
	}
	// fetch object from the database
	Blobarray := []*pb.BridgePort{}
	for _, port := range s.Ports {
		r := protoClone(port)
		r.Status = &pb.BridgePortStatus{OperStatus: pb.BPOperStatus_BP_OPER_STATUS_UP}
		Blobarray = append(Blobarray, r)
	}
	// sort is needed, since MAP is unsorted in golang, and we might get different results
	sortBridgePorts(Blobarray)
	log.Printf("Limiting result len(%d) to [%d:%d]", len(Blobarray), offset, size)
	Blobarray, hasMoreElements := limitPagination(Blobarray, offset, size)
	token := ""
	if hasMoreElements {
		token = uuid.New().String()
		s.Pagination[token] = offset + size
	}
	return &pb.ListBridgePortsResponse{BridgePorts: Blobarray, NextPageToken: token}, nil
}
