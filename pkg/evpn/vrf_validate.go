// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/resourceid"

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
	// TODO: add more checks here
	return nil
}
