// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"fmt"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

func (s *Server) frrCreateSviRequest(ctx context.Context, in *pb.CreateSviRequest, vrfName, vlanName string) error {
	if in.Svi.Spec.EnableBgp {
		fmt.Printf("TODO: add configuration for <%s:%s> here, see issue #233", vrfName, vlanName)
		// data, err := utils.FrrBgpCmd(ctx, fmt.Sprintf(
		// 	`configure terminal
		// 	router bgp 65000 vrf %s
		// 	bgp disable-ebgp-connected-route-check" \
		// 	neighbor %s peer-group" \
		// 	neighbor %s remote-as %s" \
		// 	neighbor %s update-source %s" \
		// 	neighbor %s as-override" \
		// 	neighbor %s soft-reconfiguration inbound" \
		// 	bgp listen range <svi-ip-with prefixlength> peer-group %s" \
		// 	exit`, vrfName, vlanName, in.Svi.Spec.RemoteAs, in.Svi.Spec.GwIpPrefix))
		// fmt.Printf("FrrBgpCmd: %v:%v", data, err)
		// if err != nil {
		// 	return err
		// }
	}
	// check FRR for debug
	data, err := utils.FrrZebraCmd(ctx, "show vrf")
	fmt.Printf("FrrZebraCmd: %v:%v", data, err)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) frrDeleteSviRequest(ctx context.Context, obj *pb.Svi, vrfName, vlanName string) error {
	if obj.Spec.EnableBgp {
		data, err := utils.FrrBgpCmd(ctx, fmt.Sprintf(
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
