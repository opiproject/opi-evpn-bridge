// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package bridge is the main package of the application
package bridge

import (
	"context"
	"fmt"
	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/models"
	"github.com/opiproject/opi-evpn-bridge/pkg/objects"
	"log"

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
	in.LogicalBridge.Name = objects.ResourceIDToFullName(Prefix, resourceID)

	// idempotent API when called with same key, should return same object
	obj, ok, err := s.Get(in.LogicalBridge.Name)
	if err != nil {
		return nil, err
	}
	if ok {
		log.Printf("Already existing LogicalBridge with id %v", in.LogicalBridge.Name)
		return (*obj).ToPb(), nil
	}
	// configure netlink
	if err := s.netlinkCreateLogicalBridge(ctx, in); err != nil {
		return nil, err
	}
	// translate object
	val := models.NewBridge(in.LogicalBridge)
	// save object to the database
	err = s.Create(&val)
	if err != nil {
		return nil, err
	}

	return val.ToPb(), nil
}

// DeleteLogicalBridge deletes a LogicalBridge
func (s *Server) DeleteLogicalBridge(ctx context.Context, in *pb.DeleteLogicalBridgeRequest) (*emptypb.Empty, error) {
	// check input correctness
	if err := s.validateDeleteLogicalBridgeRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	obj, ok, err := s.Get(in.Name)
	if err != nil {
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
	if err := s.netlinkDeleteLogicalBridge(ctx, (*obj).ToPb()); err != nil {
		return nil, err
	}
	// remove from the Database
	err = s.Delete(obj)
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
	obj, ok, err := s.Get(in.LogicalBridge.Name)
	if err != nil {
		return nil, err
	}
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.LogicalBridge.Name)
		return nil, err
	}

	// only if VNI is not empty
	vni := (*obj).ToPb().GetSpec().Vni
	if vni != nil {
		vxlanName := fmt.Sprintf("vni: %d", vni)
		iface, err := s.NLink.LinkByName(ctx, vxlanName)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", vxlanName)
			return nil, err
		}
		// base := iface.Attrs()
		// iface.MTU = 1500 // TODO: remove this, just an example
		if err := s.NLink.LinkModify(ctx, iface); err != nil {
			fmt.Printf("Failed to update link: %v", err)
			return nil, err
		}
	}

	newBridge := models.NewBridge(in.LogicalBridge)
	err = s.Set(obj, &newBridge)
	if err != nil {
		return nil, err
	}
	return newBridge.ToPb(), nil
}

// GetLogicalBridge gets a LogicalBridge
func (s *Server) GetLogicalBridge(ctx context.Context, in *pb.GetLogicalBridgeRequest) (*pb.LogicalBridge, error) {
	// check input correctness
	if err := s.validateGetLogicalBridgeRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	obj, _, err := s.Get(in.Name)
	if err != nil {
		return nil, err
	}

	// only if VNI is not empty
	toPb := (*obj).ToPb()
	vni := toPb.GetSpec().Vni
	if vni != nil {
		vxlanName := fmt.Sprintf("vni: %d", vni)
		_, err := s.NLink.LinkByName(ctx, vxlanName)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", vxlanName)
			return nil, err
		}
	}

	return toPb, nil
}

// ListLogicalBridges lists logical bridges
func (s *Server) ListLogicalBridges(_ context.Context, in *pb.ListLogicalBridgesRequest) (*pb.ListLogicalBridgesResponse, error) {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return nil, err
	}

	list, token, err := s.List(in.PageSize, in.PageToken, Prefix)
	if err != nil {
		return nil, err
	}

	return &pb.ListLogicalBridgesResponse{LogicalBridges: list, NextPageToken: token}, nil
}
