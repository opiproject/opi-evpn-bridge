// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"fmt"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/resourceid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

func (s *Server) validateCreateLogicalBridgeRequest(in *pb.CreateLogicalBridgeRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// check vlan id is in range
	if in.LogicalBridge.Spec.VlanId > 4095 {
		msg := fmt.Sprintf("VlanId value (%d) have to be between 1 and 4095", in.LogicalBridge.Spec.VlanId)
		return status.Errorf(codes.InvalidArgument, msg)
	}
	// see https://google.aip.dev/133#user-specified-ids
	if in.LogicalBridgeId != "" {
		if err := resourceid.ValidateUserSettable(in.LogicalBridgeId); err != nil {
			return err
		}
	}
	return nil
}
