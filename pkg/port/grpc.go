// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package port is the main package of the application
package port

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/opiproject/opi-evpn-bridge/pkg/models"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/resourceid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

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
	obj := new(pb.BridgePort)
	ok, err := s.store.Get(in.BridgePort.Name, obj)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if ok {
		log.Printf("Already existing BridgePort with id %v", in.BridgePort.Name)
		return obj, nil
	}
	// configure netlink
	if err := s.netlinkCreateBridgePort(ctx, in); err != nil {
		return nil, err
	}
	// translate object
	response := protoClone(in.BridgePort)
	response.Status = &pb.BridgePortStatus{OperStatus: pb.BPOperStatus_BP_OPER_STATUS_UP}
	log.Printf("new object %v", models.NewPort(response))
	// save object to the database
	s.ListHelper[in.BridgePort.Name] = false
	err = s.store.Set(in.BridgePort.Name, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// DeleteBridgePort deletes a port
func (s *Server) DeleteBridgePort(ctx context.Context, in *pb.DeleteBridgePortRequest) (*emptypb.Empty, error) {
	// check input correctness
	if err := s.validateDeleteBridgePortRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	iface := new(pb.BridgePort)
	ok, err := s.store.Get(in.Name, iface)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return nil, err
	}
	// configure netlink
	if err := s.netlinkDeleteBridgePort(ctx, iface); err != nil {
		return nil, err
	}
	// remove from the Database
	delete(s.ListHelper, iface.Name)
	err = s.store.Delete(iface.Name)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// UpdateBridgePort updates an Nvme Subsystem
func (s *Server) UpdateBridgePort(ctx context.Context, in *pb.UpdateBridgePortRequest) (*pb.BridgePort, error) {
	// check input correctness
	if err := s.validateUpdateBridgePortRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the
	port := new(pb.BridgePort)
	ok, err := s.store.Get(in.BridgePort.Name, port)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
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
	err = s.store.Set(in.BridgePort.Name, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetBridgePort gets an BridgePort
func (s *Server) GetBridgePort(ctx context.Context, in *pb.GetBridgePortRequest) (*pb.BridgePort, error) {
	// check input correctness
	if err := s.validateGetBridgePortRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	port := new(pb.BridgePort)
	ok, err := s.store.Get(in.Name, port)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return nil, err
	}
	resourceID := path.Base(port.Name)
	_, err = s.nLink.LinkByName(ctx, resourceID)
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
	for key := range s.ListHelper {
		if !strings.HasPrefix(key, "//network.opiproject.org/ports") {
			continue
		}
		port := new(pb.BridgePort)
		ok, err := s.store.Get(key, port)
		if err != nil {
			fmt.Printf("Failed to interact with store: %v", err)
			return nil, err
		}
		if !ok {
			err := status.Errorf(codes.NotFound, "unable to find key %s", key)
			return nil, err
		}
		Blobarray = append(Blobarray, port)
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
