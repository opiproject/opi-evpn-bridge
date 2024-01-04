// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package port is the main package of the application
package port

import (
	"fmt"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/resourceid"
	"go.einride.tech/aip/resourcename"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

func (s *Server) validateCreateBridgePortRequest(in *pb.CreateBridgePortRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// see https://google.aip.dev/133#user-specified-ids
	if in.BridgePortId != "" {
		if err := resourceid.ValidateUserSettable(in.BridgePortId); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) validateBridgePortSpec(bp *pb.BridgePort) error {
	// Validate that a LogicalBridge resource name conforms to the restrictions outlined in AIP-122.
	if bp.Spec.LogicalBridges != nil {
		for _, lb := range bp.Spec.LogicalBridges {
			if err := resourcename.Validate(lb); err != nil {
				msg := fmt.Sprintf("Logical Bridge %v has invalid name, error: %v", lb, err)
				return status.Errorf(codes.InvalidArgument, msg)
			}
		}
	}

	// for Access type, the LogicalBridge list must have only one item
	if bp.Spec.Ptype == pb.BridgePortType_BRIDGE_PORT_TYPE_ACCESS {
		if bp.Spec.LogicalBridges == nil {
			msg := "LogicalBridges field cannot be empty when the Bridge Port is of type ACCESS"
			return status.Errorf(codes.InvalidArgument, msg)
		}

		lenLbs := len(bp.Spec.LogicalBridges)
		if lenLbs > 1 {
			msg := fmt.Sprintf("ACCESS type must have single LogicalBridge and not (%d)", lenLbs)
			return status.Errorf(codes.InvalidArgument, msg)
		}
	}

	// validate MacAddress format
	if err := utils.ValidateMacAddress(bp.Spec.MacAddress); err != nil {
		msg := fmt.Sprintf("Invalid format of MAC Address: %v", err)
		return status.Errorf(codes.InvalidArgument, msg)
	}

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

func (s *Server) validateListBridgePortsRequest(in *pb.ListBridgePortsRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	return nil
}
