// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"fmt"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

func (s *Server) frrCreateSviRequest(ctx context.Context, in *pb.CreateSviRequest, vrfName, vlanName string) error {
	if in.Svi.Spec.EnableBgp {
		data, err := s.frr.FrrBgpCmd(ctx, fmt.Sprintf(
			`configure terminal
			router bgp 65000 vrf %[1]s
			bgp disable-ebgp-connected-route-check" \
			neighbor %[2]s peer-group" \
			neighbor %[2]s remote-as %[3]d" \
			neighbor %[2]s as-override" \
			neighbor %[2]s soft-reconfiguration inbound" \
			exit`, vrfName, vlanName, in.Svi.Spec.RemoteAs))
		// TODO: see issue #233, add "neighbor update-source" and "bgp listen range" with in.Svi.Spec.GwIpPrefix
		fmt.Printf("FrrBgpCmd: %v:%v", data, err)
		if err != nil {
			return err
		}
	}
	// check FRR for debug
	data, err := s.frr.FrrZebraCmd(ctx, "show vrf")
	fmt.Printf("FrrZebraCmd: %v:%v", data, err)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) frrDeleteSviRequest(ctx context.Context, obj *pb.Svi, vrfName, vlanName string) error {
	if obj.Spec.EnableBgp {
		data, err := s.frr.FrrBgpCmd(ctx, fmt.Sprintf(
			`configure terminal
			router bgp 65000 vrf %s
			no neighbor %s peer-group
			exit`, vrfName, vlanName))
		fmt.Printf("FrrBgpCmd: %v:%v", data, err)
		if err != nil {
			return err
		}
	}
	return nil
}
