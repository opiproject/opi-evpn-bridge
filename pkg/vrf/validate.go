// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package vrf is the main package of the application
package vrf

import (
	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/resourceid"
	"go.einride.tech/aip/resourcename"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

func (s *Server) validateCreateVrfRequest(in *pb.CreateVrfRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// see https://google.aip.dev/133#user-specified-ids
	if in.VrfId != "" {
		if err := resourceid.ValidateUserSettable(in.VrfId); err != nil {
			return err
		}
	}
	// TODO: check in.Vrf.Spec.Vni validity
	return nil
}

func (s *Server) validateDeleteVrfRequest(in *pb.DeleteVrfRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return resourcename.Validate(in.Name)
}

func (s *Server) validateUpdateVrfRequest(in *pb.UpdateVrfRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.Vrf); err != nil {
		return err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return resourcename.Validate(in.Vrf.Name)
}

func (s *Server) validateGetVrfRequest(in *pb.GetVrfRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	return resourcename.Validate(in.Name)
}
