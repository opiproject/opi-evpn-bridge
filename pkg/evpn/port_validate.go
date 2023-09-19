// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"fmt"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/resourceid"
	"go.einride.tech/aip/resourcename"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

func (s *Server) validateCreateBridgePortRequest(in *pb.CreateBridgePortRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// for Access type, the LogicalBridge list must have only one item
	length := len(in.BridgePort.Spec.LogicalBridges)
	if in.BridgePort.Spec.Ptype == pb.BridgePortType_ACCESS && length > 1 {
		msg := fmt.Sprintf("ACCESS type must have single LogicalBridge and not (%d)", length)
		return status.Errorf(codes.InvalidArgument, msg)
	}
	// see https://google.aip.dev/133#user-specified-ids
	if in.BridgePortId != "" {
		if err := resourceid.ValidateUserSettable(in.BridgePortId); err != nil {
			return err
		}
	}
	// TODO: check in.BridgePort.Spec.MacAddress validity
	return nil
}

func (s *Server) validateDeleteBridgePortRequest(in *pb.DeleteBridgePortRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return resourcename.Validate(in.Name)
}

func (s *Server) validateUpdateBridgePortRequest(in *pb.UpdateBridgePortRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.BridgePort); err != nil {
		return err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return resourcename.Validate(in.BridgePort.Name)
}

func (s *Server) validateGetBridgePortRequest(in *pb.GetBridgePortRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return resourcename.Validate(in.Name)
}
