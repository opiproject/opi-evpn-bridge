// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package bridge is the main package of the application
package bridge

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/opiproject/opi-evpn-bridge/pkg/models"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/resourceid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// CreateLogicalBridge executes the creation of the LogicalBridge
func (s *Server) CreateLogicalBridge(ctx context.Context, in *pb.CreateLogicalBridgeRequest) (*pb.LogicalBridge, error) {
	// check input correctness
	if err := s.validateCreateLogicalBridgeRequest(in); err != nil {
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.LogicalBridgeId != "" {
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.LogicalBridgeId, in.LogicalBridge.Name)
		resourceID = in.LogicalBridgeId
	}
	in.LogicalBridge.Name = resourceIDToFullName("bridges", resourceID)
	// idempotent API when called with same key, should return same object
	obj := new(pb.LogicalBridge)
	ok, err := s.store.Get(in.LogicalBridge.Name, obj)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if ok {
		log.Printf("Already existing LogicalBridge with id %v", in.LogicalBridge.Name)
		return obj, nil
	}
	// configure netlink
	if err := s.netlinkCreateLogicalBridge(ctx, in); err != nil {
		return nil, err
	}
	// translate object
	response := utils.ProtoClone(in.LogicalBridge)
	response.Status = &pb.LogicalBridgeStatus{OperStatus: pb.LBOperStatus_LB_OPER_STATUS_UP}
	log.Printf("new object %v", models.NewBridge(response))
	// save object to the database
	s.ListHelper[in.LogicalBridge.Name] = false
	err = s.store.Set(in.LogicalBridge.Name, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// DeleteLogicalBridge deletes a LogicalBridge
func (s *Server) DeleteLogicalBridge(ctx context.Context, in *pb.DeleteLogicalBridgeRequest) (*emptypb.Empty, error) {
	// check input correctness
	if err := s.validateDeleteLogicalBridgeRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	obj := new(pb.LogicalBridge)
	ok, err := s.store.Get(in.Name, obj)
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
	if err := s.netlinkDeleteLogicalBridge(ctx, obj); err != nil {
		return nil, err
	}
	// remove from the Database
	delete(s.ListHelper, obj.Name)
	err = s.store.Delete(obj.Name)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// UpdateLogicalBridge updates a LogicalBridge
func (s *Server) UpdateLogicalBridge(ctx context.Context, in *pb.UpdateLogicalBridgeRequest) (*pb.LogicalBridge, error) {
	// check input correctness
	if err := s.validateUpdateLogicalBridgeRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	bridge := new(pb.LogicalBridge)
	ok, err := s.store.Get(in.LogicalBridge.Name, bridge)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.LogicalBridge.Name)
		return nil, err
	}
	// only if VNI is not empty
	if bridge.Spec.Vni != nil {
		vxlanName := fmt.Sprintf("vni%d", *bridge.Spec.Vni)
		iface, err := s.nLink.LinkByName(ctx, vxlanName)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", vxlanName)
			return nil, err
		}
		// base := iface.Attrs()
		// iface.MTU = 1500 // TODO: remove this, just an example
		if err := s.nLink.LinkModify(ctx, iface); err != nil {
			fmt.Printf("Failed to update link: %v", err)
			return nil, err
		}
	}
	response := utils.ProtoClone(in.LogicalBridge)
	response.Status = &pb.LogicalBridgeStatus{OperStatus: pb.LBOperStatus_LB_OPER_STATUS_UP}
	err = s.store.Set(in.LogicalBridge.Name, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetLogicalBridge gets a LogicalBridge
func (s *Server) GetLogicalBridge(ctx context.Context, in *pb.GetLogicalBridgeRequest) (*pb.LogicalBridge, error) {
	// check input correctness
	if err := s.validateGetLogicalBridgeRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	bridge := new(pb.LogicalBridge)
	ok, err := s.store.Get(in.Name, bridge)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return nil, err
	}
	// only if VNI is not empty
	if bridge.Spec.Vni != nil {
		vxlanName := fmt.Sprintf("vni%d", *bridge.Spec.Vni)
		_, err := s.nLink.LinkByName(ctx, vxlanName)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", vxlanName)
			return nil, err
		}
	}
	// TODO
	return &pb.LogicalBridge{Name: in.Name, Spec: &pb.LogicalBridgeSpec{Vni: bridge.Spec.Vni, VlanId: bridge.Spec.VlanId}, Status: &pb.LogicalBridgeStatus{OperStatus: pb.LBOperStatus_LB_OPER_STATUS_UP}}, nil
}

// ListLogicalBridges lists logical bridges
func (s *Server) ListLogicalBridges(_ context.Context, in *pb.ListLogicalBridgesRequest) (*pb.ListLogicalBridgesResponse, error) {
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
	Blobarray := []*pb.LogicalBridge{}
	for key := range s.ListHelper {
		if !strings.HasPrefix(key, "//network.opiproject.org/bridges") {
			continue
		}
		bridge := new(pb.LogicalBridge)
		ok, err := s.store.Get(key, bridge)
		if err != nil {
			fmt.Printf("Failed to interact with store: %v", err)
			return nil, err
		}
		if !ok {
			err := status.Errorf(codes.NotFound, "unable to find key %s", key)
			return nil, err
		}
		Blobarray = append(Blobarray, bridge)
	}
	// sort is needed, since MAP is unsorted in golang, and we might get different results
	sortLogicalBridges(Blobarray)
	log.Printf("Limiting result len(%d) to [%d:%d]", len(Blobarray), offset, size)
	Blobarray, hasMoreElements := limitPagination(Blobarray, offset, size)
	token := ""
	if hasMoreElements {
		token = uuid.New().String()
		s.Pagination[token] = offset + size
	}
	return &pb.ListLogicalBridgesResponse{LogicalBridges: Blobarray, NextPageToken: token}, nil
}
