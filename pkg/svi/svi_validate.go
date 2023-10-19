// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package svi is the main package of the application
package svi

import (
	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/resourceid"
	"go.einride.tech/aip/resourcename"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

func (s *Server) validateCreateSviRequest(in *pb.CreateSviRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// Validate that a LogicalBridge resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Svi.Spec.LogicalBridge); err != nil {
		return err
	}
	// Validate that a Vrf resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Svi.Spec.Vrf); err != nil {
		return err
	}
	// see https://google.aip.dev/133#user-specified-ids
	if in.SviId != "" {
		if err := resourceid.ValidateUserSettable(in.SviId); err != nil {
			return err
		}
	}
	// TODO: check in.Svi.Spec.MacAddress validity
	return nil
}

func (s *Server) validateDeleteSviRequest(in *pb.DeleteSviRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return resourcename.Validate(in.Name)
}

func (s *Server) validateUpdateSviRequest(in *pb.UpdateSviRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.Svi); err != nil {
		return err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return resourcename.Validate(in.Svi.Name)
}

func (s *Server) validateGetSviRequest(in *pb.GetSviRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return resourcename.Validate(in.Name)
}
