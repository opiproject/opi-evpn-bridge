// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package svi is the main package of the application
package svi

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
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

func (s *Server) validateCreateSviRequest(in *pb.CreateSviRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}

	// see https://google.aip.dev/133#user-specified-ids
	if in.SviId != "" {
		if err := resourceid.ValidateUserSettable(in.SviId); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) validateSviSpec(svi *pb.Svi) error {
	// Validate that a LogicalBridge resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(svi.Spec.LogicalBridge); err != nil {
		msg := fmt.Sprintf("Logical Bridge %v has invalid name, error: %v", svi.Spec.LogicalBridge, err)
		return status.Errorf(codes.InvalidArgument, msg)
	}

	// Validate that a Vrf resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(svi.Spec.Vrf); err != nil {
		msg := fmt.Sprintf("VRF %v has invalid name, error: %v", svi.Spec.Vrf, err)
		return status.Errorf(codes.InvalidArgument, msg)
	}

	// Validate that the MacAddress has the right format
	if err := utils.ValidateMacAddress(svi.Spec.MacAddress); err != nil {
		msg := fmt.Sprintf("Invalid format of MAC Address: %v", err)
		return status.Errorf(codes.InvalidArgument, msg)
	}

	// Dimitris: Should I validate also the gw_ip_prefix ?

	// Dimitris: Do we need to change the type of RemoteAs to something else than uint32 ?
	// because now the default value is "0" which is not good. I think "optional uint32" in protobuf is better
	if svi.Spec.EnableBgp {
		if err := validateASN(svi.Spec.RemoteAs); err != nil {
			msg := fmt.Sprintf("Invalid RemoteAs: %v", err)
			return status.Errorf(codes.InvalidArgument, msg)
		}
	} else {
		if svi.Spec.RemoteAs != 0 {
			msg := "Invalid RemoteAs: RemoteAs must not be defined when EnableBgp is False"
			return status.Errorf(codes.InvalidArgument, msg)
		}
	}

	return nil
}

func validateASN(asn uint32) error {
	if asn < 1 || asn > 65535 {
		return errors.New("ASN must be in range of 1-65535")
	}
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

func (s *Server) validateListSvisRequest(in *pb.ListSvisRequest) error {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return err
	}
	return nil
}
