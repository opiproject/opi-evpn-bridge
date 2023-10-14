// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"fmt"
	"path"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

func (s *Server) frrCreateVrfRequest(ctx context.Context, in *pb.CreateVrfRequest) error {
	vrfName := path.Base(in.Vrf.Name)
	if in.Vrf.Spec.Vni != nil {
		data, err := utils.FrrZebraCmd(ctx, fmt.Sprintf(
			`configure terminal
			vrf %s
				vni %d
				exit-vrf
			exit`, vrfName, *in.Vrf.Spec.Vni))
		fmt.Printf("FrrZebraCmd: %v:%v", data, err)
		if err != nil {
			return err
		}
	}
	if in.Vrf.Spec.Vni != nil {
		data, err := utils.FrrBgpCmd(ctx, fmt.Sprintf(
			`configure terminal
			router bgp 65000 vrf %s
			no bgp log-neighbor-changes
			bgp ebgp-requires-policy
			no bgp default show-hostname
			no bgp default show-nexthop-hostname
			no bgp deterministic-med
			timers bgp 60 180
			address-family ipv4 unicast
				redistribute kernel
				redistribute connected
				redistribute static
				maximum-paths ibgp 1
				exit-address-family
			address-family l2vpn evpn
				advertise ipv4 unicast
				exit-address-family
			exit`, vrfName))
		fmt.Printf("FrrBgpCmd: %v:%v", data, err)
		if err != nil {
			return err
		}
	}
	// check FRR for debug
	data, err := utils.FrrZebraCmd(ctx, "show vrf")
	fmt.Printf("FrrZebraCmd: %v:%v", data, err)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) frrDeleteVrfRequest(ctx context.Context, obj *pb.Vrf) error {
	vrfName := path.Base(obj.Name)
	if obj.Spec.Vni != nil {
		data, err := utils.FrrBgpCmd(ctx, fmt.Sprintf(
			`configure terminal
			no router bgp 65000 vrf %s
			exit`, vrfName))
		fmt.Printf("FrrBgpCmd: %v:%v", data, err)
		if err != nil {
			return err
		}
	}
	if obj.Spec.Vni != nil {
		data, err := utils.FrrZebraCmd(ctx, fmt.Sprintf(
			`configure terminal
			no vrf %s
			exit`, vrfName))
		fmt.Printf("FrrZebraCmd: %v:%v", data, err)
		if err != nil {
			return err
		}
	}
	return nil
}
