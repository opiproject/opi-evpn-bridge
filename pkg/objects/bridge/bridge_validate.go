// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package bridge is the main package of the application
package bridge

import (
	"errors"
	"fmt"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/resourceid"
	"go.einride.tech/aip/resourcename"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

func (s *Server) validateCreateLogicalBridgeRequest(i any) (*pb.CreateLogicalBridgeRequest, error) {
	// check required fields
	in, ok := i.(*pb.CreateLogicalBridgeRequest)
	if ok {
		return nil, errors.New("protobuf type assertion error for CreateLogicalBridgeRequest")
	}

	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return nil, err
	}
	// check vlan id is in range
	if in.LogicalBridge.Spec.VlanId > 4095 {
		msg := fmt.Sprintf("VlanId value (%d) have to be between 1 and 4095", in.LogicalBridge.Spec.VlanId)
		return nil, status.Errorf(codes.InvalidArgument, msg)
	}
	// see https://google.aip.dev/133#user-specified-ids
	if in.LogicalBridgeId != "" {
		if err := resourceid.ValidateUserSettable(in.LogicalBridgeId); err != nil {
			return nil, err
		}
	}
	// TODO: check in.LogicalBridge.Spec.Vni validity
	return in, nil
}

func (s *Server) validateDeleteLogicalBridgeRequest(i any) (*pb.DeleteLogicalBridgeRequest, error) {
	// check required fields
	in, ok := i.(*pb.DeleteLogicalBridgeRequest)
	if ok {
		return nil, errors.New("protobuf type assertion error for DeleteLogicalBridgeRequest")
	}
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return in, resourcename.Validate(in.Name)
}

func (s *Server) validateUpdateLogicalBridgeRequest(i any) (*pb.UpdateLogicalBridgeRequest, error) {
	// check required fields
	in, ok := i.(*pb.UpdateLogicalBridgeRequest)
	if ok {
		return nil, errors.New("protobuf type assertion error for UpdateLogicalBridgeRequest")
	}
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return nil, err
	}
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.LogicalBridge); err != nil {
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return in, resourcename.Validate(in.LogicalBridge.Name)
}

func (s *Server) validateGetLogicalBridgeRequest(i any) (*pb.GetLogicalBridgeRequest, error) {
	// check required fields
	in, ok := i.(*pb.GetLogicalBridgeRequest)
	if ok {
		return nil, errors.New("protobuf type assertion error for GetLogicalBridgeRequest")
	}
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return in, resourcename.Validate(in.Name)
}

func (s *Server) validateListLogicalBridgeRequest(i any) (*pb.ListLogicalBridgesRequest, error) {
	// check required fields
	in, ok := i.(*pb.ListLogicalBridgesRequest)
	if ok {
		return nil, errors.New("protobuf type assertion error for ListLogicalBridgesRequest")
	}
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return in, nil
}
