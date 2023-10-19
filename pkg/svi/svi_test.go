// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package svi is the main package of the application
package svi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"testing"

	"github.com/philippgille/gokv/gomap"
	"github.com/stretchr/testify/mock"
	"github.com/vishvananda/netlink"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	pc "github.com/opiproject/opi-api/network/opinetcommon/v1alpha1/gen/go"

	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils/mocks"
)

var (
	testSviID   = "opi-svi8"
	testSviName = resourceIDToFullName(testSviID)
	testSvi     = pb.Svi{
		Spec: &pb.SviSpec{
			Vrf:           testVrfName,
			LogicalBridge: testLogicalBridgeName,
			MacAddress:    []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
			GwIpPrefix:    []*pc.IPPrefix{{Len: 24}},
		},
	}
	testSviWithStatus = pb.Svi{
		Name: testSviName,
		Spec: testSvi.Spec,
		Status: &pb.SviStatus{
			OperStatus: pb.SVIOperStatus_SVI_OPER_STATUS_UP,
		},
	}
)

func Test_CreateSvi(t *testing.T) {
	tests := map[string]struct {
		id      string
		in      *pb.Svi
		out     *pb.Svi
		errCode codes.Code
		errMsg  string
		exist   bool
		on      func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string)
	}{
		"illegal resource_id": {
			id:      "CapitalLettersNotAllowed",
			in:      &testSvi,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  fmt.Sprintf("user-settable ID must only contain lowercase, numbers and hyphens (%v)", "got: 'C' in position 0"),
			exist:   false,
			on:      nil,
		},
		"already exists": {
			id:      testSviID,
			in:      &testSvi,
			out:     &testSviWithStatus,
			errCode: codes.OK,
			errMsg:  "",
			exist:   true,
			on:      nil,
		},
		"no required svi field": {
			id:      testSviID,
			in:      nil,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: svi",
			exist:   false,
			on:      nil,
		},
		"no required vrf field": {
			id: testSviID,
			in: &pb.Svi{
				Spec: &pb.SviSpec{},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: svi.spec.vrf",
			exist:   false,
			on:      nil,
		},
		"no required bridge field": {
			id: testSviID,
			in: &pb.Svi{
				Spec: &pb.SviSpec{
					Vrf: testVrfName,
				},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: svi.spec.logical_bridge",
			exist:   false,
			on:      nil,
		},
		"no required mac field": {
			id: testSviID,
			in: &pb.Svi{
				Spec: &pb.SviSpec{
					Vrf:           testVrfName,
					LogicalBridge: testLogicalBridgeName,
				},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: svi.spec.mac_address",
			exist:   false,
			on:      nil,
		},
		"no required gw ip field": {
			id: testSviID,
			in: &pb.Svi{
				Spec: &pb.SviSpec{
					Vrf:           testVrfName,
					LogicalBridge: testLogicalBridgeName,
					MacAddress:    []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
				},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: svi.spec.gw_ip_prefix",
			exist:   false,
			on:      nil,
		},
		"malformed LogicalBridge name": {
			id: testSviID,
			in: &pb.Svi{
				Spec: &pb.SviSpec{
					Vrf:           testVrfName,
					LogicalBridge: "-ABC-DEF",
					MacAddress:    []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
					GwIpPrefix:    []*pc.IPPrefix{{Len: 24}},
				},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  fmt.Sprintf("segment '%s': not a valid DNS name", "-ABC-DEF"),
			exist:   false,
			on:      nil,
		},
		"malformed Vrf name": {
			id: testSviID,
			in: &pb.Svi{
				Spec: &pb.SviSpec{
					Vrf:           "-ABC-DEF",
					LogicalBridge: testLogicalBridgeName,
					MacAddress:    []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
					GwIpPrefix:    []*pc.IPPrefix{{Len: 24}},
				},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  fmt.Sprintf("segment '%s': not a valid DNS name", "-ABC-DEF"),
			exist:   false,
			on:      nil,
		},
		"missing LogicalBridge name": {
			id: testSviID,
			in: &pb.Svi{
				Spec: &pb.SviSpec{
					Vrf:           testVrfName,
					LogicalBridge: "unknown-bridge-id",
					MacAddress:    []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
					GwIpPrefix:    []*pc.IPPrefix{{Len: 24}},
				},
			},
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", "unknown-bridge-id"),
			exist:   false,
			on:      nil,
		},
		"missing Vrf name": {
			id: testSviID,
			in: &pb.Svi{
				Spec: &pb.SviSpec{
					Vrf:           "unknown-vrf-id",
					LogicalBridge: testLogicalBridgeName,
					MacAddress:    []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
					GwIpPrefix:    []*pc.IPPrefix{{Len: 24}},
				},
			},
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", "unknown-vrf-id"),
			exist:   false,
			on:      nil,
		},
		"failed bridge LinkByName call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", tenantbridgeName),
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed BridgeVlanAdd call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call BridgeVlanAdd",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				vid := uint16(testLogicalBridge.Spec.VlanId)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, bridge, vid, false, false, true, false).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkAdd call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkAdd",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				vid := uint16(testLogicalBridge.Spec.VlanId)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, bridge, vid, false, false, true, false).Return(nil).Once()
				vlanName := fmt.Sprintf("vlan%d", vid)
				vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vlandev).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetHardwareAddr call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetHardwareAddr",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				vid := uint16(testLogicalBridge.Spec.VlanId)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, bridge, vid, false, false, true, false).Return(nil).Once()
				vlanName := fmt.Sprintf("vlan%d", vid)
				vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vlandev).Return(nil).Once()
				mac := net.HardwareAddr(testSvi.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(mock.Anything, vlandev, mac).Return(errors.New(errMsg)).Once()
			},
		},
		"failed AddrAdd call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call AddrAdd",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				vid := uint16(testLogicalBridge.Spec.VlanId)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, bridge, vid, false, false, true, false).Return(nil).Once()
				vlanName := fmt.Sprintf("vlan%d", vid)
				vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vlandev).Return(nil).Once()
				mac := net.HardwareAddr(testSvi.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(mock.Anything, vlandev, mac).Return(nil).Once()
				myip := make(net.IP, 4)
				addr := &netlink.Addr{IPNet: &net.IPNet{IP: myip, Mask: net.CIDRMask(24, 32)}}
				mockNetlink.EXPECT().AddrAdd(mock.Anything, vlandev, addr).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkByName call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", testVrfName),
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				vid := uint16(testLogicalBridge.Spec.VlanId)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, bridge, vid, false, false, true, false).Return(nil).Once()
				vlanName := fmt.Sprintf("vlan%d", vid)
				vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vlandev).Return(nil).Once()
				mac := net.HardwareAddr(testSvi.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(mock.Anything, vlandev, mac).Return(nil).Once()
				myip := make(net.IP, 4)
				addr := &netlink.Addr{IPNet: &net.IPNet{IP: myip, Mask: net.CIDRMask(24, 32)}}
				mockNetlink.EXPECT().AddrAdd(mock.Anything, vlandev, addr).Return(nil).Once()
				mockNetlink.EXPECT().LinkByName(mock.Anything, testVrfID).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetMaster call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetMaster",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				vid := uint16(testLogicalBridge.Spec.VlanId)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, bridge, vid, false, false, true, false).Return(nil).Once()
				vlanName := fmt.Sprintf("vlan%d", vid)
				vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vlandev).Return(nil).Once()
				mac := net.HardwareAddr(testSvi.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(mock.Anything, vlandev, mac).Return(nil).Once()
				myip := make(net.IP, 4)
				addr := &netlink.Addr{IPNet: &net.IPNet{IP: myip, Mask: net.CIDRMask(24, 32)}}
				mockNetlink.EXPECT().AddrAdd(mock.Anything, vlandev, addr).Return(nil).Once()
				vrfdev := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				mockNetlink.EXPECT().LinkByName(mock.Anything, testVrfID).Return(vrfdev, nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(mock.Anything, vlandev, vrfdev).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetUp call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetUp",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				vid := uint16(testLogicalBridge.Spec.VlanId)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, bridge, vid, false, false, true, false).Return(nil).Once()
				vlanName := fmt.Sprintf("vlan%d", vid)
				vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vlandev).Return(nil).Once()
				mac := net.HardwareAddr(testSvi.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(mock.Anything, vlandev, mac).Return(nil).Once()
				myip := make(net.IP, 4)
				addr := &netlink.Addr{IPNet: &net.IPNet{IP: myip, Mask: net.CIDRMask(24, 32)}}
				mockNetlink.EXPECT().AddrAdd(mock.Anything, vlandev, addr).Return(nil).Once()
				vrfdev := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				mockNetlink.EXPECT().LinkByName(mock.Anything, testVrfID).Return(vrfdev, nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(mock.Anything, vlandev, vrfdev).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(mock.Anything, vlandev).Return(errors.New(errMsg)).Once()
			},
		},
		"successful call": {
			id:      testSviID,
			in:      &testSvi,
			out:     &testSviWithStatus,
			errCode: codes.OK,
			errMsg:  "",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				vid := uint16(testLogicalBridge.Spec.VlanId)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, bridge, vid, false, false, true, false).Return(nil).Once()
				vlanName := fmt.Sprintf("vlan%d", vid)
				vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vlandev).Return(nil).Once()
				mac := net.HardwareAddr(testSvi.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(mock.Anything, vlandev, mac).Return(nil).Once()
				myip := make(net.IP, 4)
				addr := &netlink.Addr{IPNet: &net.IPNet{IP: myip, Mask: net.CIDRMask(24, 32)}}
				mockNetlink.EXPECT().AddrAdd(mock.Anything, vlandev, addr).Return(nil).Once()
				vrfdev := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				mockNetlink.EXPECT().LinkByName(mock.Anything, testVrfID).Return(vrfdev, nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(mock.Anything, vlandev, vrfdev).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(mock.Anything, vlandev).Return(nil).Once()
				// frr
				mockFrr.EXPECT().FrrZebraCmd(mock.Anything, mock.Anything).Return("", nil).Once()
			},
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			mockNetlink := mocks.NewNetlink(t)
			mockFrr := mocks.NewFrr(t)
			store := gomap.NewStore(gomap.Options{Codec: utils.ProtoCodec{}})
			opi := NewServerWithArgs(mockNetlink, mockFrr, store)
			conn, err := grpc.DialContext(ctx,
				"",
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithContextDialer(dialer(opi)))
			if err != nil {
				log.Fatal(err)
			}
			defer func(conn *grpc.ClientConn) {
				err := conn.Close()
				if err != nil {
					log.Fatal(err)
				}
			}(conn)
			client := pb.NewSviServiceClient(conn)

			if tt.exist {
				_ = opi.store.Set(testSviName, &testSviWithStatus)
			}
			if tt.out != nil {
				tt.out = utils.ProtoClone(tt.out)
				tt.out.Name = testSviName
			}
			if tt.on != nil {
				tt.on(mockNetlink, mockFrr, tt.errMsg)
			}
			_ = opi.store.Set(testVrfName, &testVrfWithStatus)
			_ = opi.store.Set(testLogicalBridgeName, &testLogicalBridgeWithStatus)

			request := &pb.CreateSviRequest{Svi: tt.in, SviId: tt.id}
			response, err := client.CreateSvi(ctx, request)
			if !proto.Equal(tt.out, response) {
				t.Error("response: expected", tt.out, "received", response)
			}

			if er, ok := status.FromError(err); ok {
				if er.Code() != tt.errCode {
					t.Error("error code: expected", tt.errCode, "received", er.Code())
				}
				if er.Message() != tt.errMsg {
					t.Error("error message: expected", tt.errMsg, "received", er.Message())
				}
			} else {
				t.Error("expected grpc error status")
			}
		})
	}
}

func Test_DeleteSvi(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     *emptypb.Empty
		errCode codes.Code
		errMsg  string
		missing bool
		on      func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string)
	}{
		"valid request with unknown key": {
			in:      "unknown-id",
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", resourceIDToFullName("unknown-id")),
			missing: false,
			on:      nil,
		},
		"unknown key with missing allowed": {
			in:      "unknown-id",
			out:     &emptypb.Empty{},
			errCode: codes.OK,
			errMsg:  "",
			missing: true,
			on:      nil,
		},
		"malformed name": {
			in:      "-ABC-DEF",
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  fmt.Sprintf("segment '%s': not a valid DNS name", "-ABC-DEF"),
			missing: false,
			on:      nil,
		},
		"failed bridge LinkByName call": {
			in:      testSviID,
			out:     &emptypb.Empty{},
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", tenantbridgeName),
			missing: false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed BridgeVlanDel call": {
			in:      testSviID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call BridgeVlanDel",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanDel(mock.Anything, bridge, vid, false, false, true, false).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkByName call": {
			in:      testSviID,
			out:     &emptypb.Empty{},
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", "vlan22"),
			missing: false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanDel(mock.Anything, bridge, vid, false, false, true, false).Return(nil).Once()
				vlanName := fmt.Sprintf("vlan%d", vid)
				mockNetlink.EXPECT().LinkByName(mock.Anything, vlanName).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetDown call": {
			in:      testSviID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetDown",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanDel(mock.Anything, bridge, vid, false, false, true, false).Return(nil).Once()
				vlanName := fmt.Sprintf("vlan%d", vid)
				vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
				mockNetlink.EXPECT().LinkByName(mock.Anything, vlanName).Return(vlandev, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(mock.Anything, vlandev).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkDel call": {
			in:      testSviID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkDel",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanDel(mock.Anything, bridge, vid, false, false, true, false).Return(nil).Once()
				vlanName := fmt.Sprintf("vlan%d", vid)
				vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
				mockNetlink.EXPECT().LinkByName(mock.Anything, vlanName).Return(vlandev, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(mock.Anything, vlandev).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(mock.Anything, vlandev).Return(errors.New(errMsg)).Once()
			},
		},
		"successful call": {
			in:      testSviID,
			out:     &emptypb.Empty{},
			errCode: codes.OK,
			errMsg:  "",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanDel(mock.Anything, bridge, vid, false, false, true, false).Return(nil).Once()
				vlanName := fmt.Sprintf("vlan%d", vid)
				vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
				mockNetlink.EXPECT().LinkByName(mock.Anything, vlanName).Return(vlandev, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(mock.Anything, vlandev).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(mock.Anything, vlandev).Return(nil).Once()
			},
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			mockNetlink := mocks.NewNetlink(t)
			mockFrr := mocks.NewFrr(t)
			store := gomap.NewStore(gomap.Options{Codec: utils.ProtoCodec{}})
			opi := NewServerWithArgs(mockNetlink, mockFrr, store)
			conn, err := grpc.DialContext(ctx,
				"",
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithContextDialer(dialer(opi)))
			if err != nil {
				log.Fatal(err)
			}
			defer func(conn *grpc.ClientConn) {
				err := conn.Close()
				if err != nil {
					log.Fatal(err)
				}
			}(conn)
			client := pb.NewSviServiceClient(conn)

			fname1 := resourceIDToFullName(tt.in)
			_ = opi.store.Set(testSviName, &testSviWithStatus)
			_ = opi.store.Set(testVrfName, &testVrfWithStatus)
			_ = opi.store.Set(testLogicalBridgeName, &testLogicalBridgeWithStatus)
			if tt.on != nil {
				tt.on(mockNetlink, mockFrr, tt.errMsg)
			}

			request := &pb.DeleteSviRequest{Name: fname1, AllowMissing: tt.missing}
			response, err := client.DeleteSvi(ctx, request)

			if er, ok := status.FromError(err); ok {
				if er.Code() != tt.errCode {
					t.Error("error code: expected", tt.errCode, "received", er.Code())
				}
				if er.Message() != tt.errMsg {
					t.Error("error message: expected", tt.errMsg, "received", er.Message())
				}
			} else {
				t.Error("expected grpc error status")
			}

			if reflect.TypeOf(response) != reflect.TypeOf(tt.out) {
				t.Error("response: expected", reflect.TypeOf(tt.out), "received", reflect.TypeOf(response))
			}
		})
	}
}

func Test_UpdateSvi(t *testing.T) {
	spec := &pb.SviSpec{
		Vrf:           testVrfName,
		LogicalBridge: testLogicalBridgeName,
		MacAddress:    []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
		GwIpPrefix:    []*pc.IPPrefix{{Len: 24}},
	}
	tests := map[string]struct {
		mask    *fieldmaskpb.FieldMask
		in      *pb.Svi
		out     *pb.Svi
		errCode codes.Code
		errMsg  string
		start   bool
		exist   bool
	}{
		"invalid fieldmask": {
			mask: &fieldmaskpb.FieldMask{Paths: []string{"*", "author"}},
			in: &pb.Svi{
				Name: testSviName,
				Spec: spec,
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  fmt.Sprintf("invalid field path: %s", "'*' must not be used with other paths"),
			start:   false,
			exist:   true,
		},
		"valid request with unknown key": {
			mask: nil,
			in: &pb.Svi{
				Name: resourceIDToFullName("unknown-id"),
				Spec: spec,
			},
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", resourceIDToFullName("unknown-id")),
			start:   false,
			exist:   true,
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			mockNetlink := mocks.NewNetlink(t)
			mockFrr := mocks.NewFrr(t)
			store := gomap.NewStore(gomap.Options{Codec: utils.ProtoCodec{}})
			opi := NewServerWithArgs(mockNetlink, mockFrr, store)
			conn, err := grpc.DialContext(ctx,
				"",
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithContextDialer(dialer(opi)))
			if err != nil {
				log.Fatal(err)
			}
			defer func(conn *grpc.ClientConn) {
				err := conn.Close()
				if err != nil {
					log.Fatal(err)
				}
			}(conn)
			client := pb.NewSviServiceClient(conn)

			if tt.exist {
				_ = opi.store.Set(testSviName, &testSviWithStatus)
			}
			if tt.out != nil {
				tt.out = utils.ProtoClone(tt.out)
				tt.out.Name = testSviName
			}

			request := &pb.UpdateSviRequest{Svi: tt.in, UpdateMask: tt.mask}
			response, err := client.UpdateSvi(ctx, request)
			if !proto.Equal(tt.out, response) {
				t.Error("response: expected", tt.out, "received", response)
			}

			if er, ok := status.FromError(err); ok {
				if er.Code() != tt.errCode {
					t.Error("error code: expected", tt.errCode, "received", er.Code())
				}
				if er.Message() != tt.errMsg {
					t.Error("error message: expected", tt.errMsg, "received", er.Message())
				}
			} else {
				t.Error("expected grpc error status")
			}
		})
	}
}

func Test_GetSvi(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     *pb.Svi
		errCode codes.Code
		errMsg  string
	}{
		// "valid request": {
		// 	in: testSviID,
		// 	out: &pb.Svi{
		// 		Name: testSviName,
		// 		Spec: testSvi.Spec,
		// 	},
		// 	errCode: codes.OK,
		// 	errMsg: "",
		// },
		"valid request with unknown key": {
			in:      "unknown-id",
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", "unknown-id"),
		},
		"malformed name": {
			in:      "-ABC-DEF",
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  fmt.Sprintf("segment '%s': not a valid DNS name", "-ABC-DEF"),
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			mockNetlink := mocks.NewNetlink(t)
			mockFrr := mocks.NewFrr(t)
			store := gomap.NewStore(gomap.Options{Codec: utils.ProtoCodec{}})
			opi := NewServerWithArgs(mockNetlink, mockFrr, store)
			conn, err := grpc.DialContext(ctx,
				"",
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithContextDialer(dialer(opi)))
			if err != nil {
				log.Fatal(err)
			}
			defer func(conn *grpc.ClientConn) {
				err := conn.Close()
				if err != nil {
					log.Fatal(err)
				}
			}(conn)
			client := pb.NewSviServiceClient(conn)

			_ = opi.store.Set(testSviName, &testSviWithStatus)

			request := &pb.GetSviRequest{Name: tt.in}
			response, err := client.GetSvi(ctx, request)
			if !proto.Equal(tt.out, response) {
				t.Error("response: expected", tt.out, "received", response)
			}

			if er, ok := status.FromError(err); ok {
				if er.Code() != tt.errCode {
					t.Error("error code: expected", tt.errCode, "received", er.Code())
				}
				if er.Message() != tt.errMsg {
					t.Error("error message: expected", tt.errMsg, "received", er.Message())
				}
			} else {
				t.Error("expected grpc error status")
			}
		})
	}
}

func Test_ListSvis(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     []*pb.Svi
		errCode codes.Code
		errMsg  string
		size    int32
		token   string
	}{
		"example test": {
			in:      "",
			out:     []*pb.Svi{&testSviWithStatus},
			errCode: codes.OK,
			errMsg:  "",
			size:    0,
			token:   "",
		},
		"pagination negative": {
			in:      "",
			out:     nil,
			errCode: codes.InvalidArgument,
			errMsg:  "negative PageSize is not allowed",
			size:    -10,
			token:   "",
		},
		"pagination error": {
			in:      "",
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find pagination token %s", "unknown-pagination-token"),
			size:    0,
			token:   "unknown-pagination-token",
		},
		"pagination overflow": {
			in:      "",
			out:     []*pb.Svi{&testSviWithStatus},
			errCode: codes.OK,
			errMsg:  "",
			size:    1000,
			token:   "",
		},
		"pagination normal": {
			in:      "",
			out:     []*pb.Svi{&testSviWithStatus},
			errCode: codes.OK,
			errMsg:  "",
			size:    1,
			token:   "",
		},
		"pagination offset": {
			in:      "",
			out:     []*pb.Svi{},
			errCode: codes.OK,
			errMsg:  "",
			size:    1,
			token:   "existing-pagination-token",
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			mockNetlink := mocks.NewNetlink(t)
			mockFrr := mocks.NewFrr(t)
			store := gomap.NewStore(gomap.Options{Codec: utils.ProtoCodec{}})
			opi := NewServerWithArgs(mockNetlink, mockFrr, store)
			conn, err := grpc.DialContext(ctx,
				"",
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithContextDialer(dialer(opi)))
			if err != nil {
				log.Fatal(err)
			}
			defer func(conn *grpc.ClientConn) {
				err := conn.Close()
				if err != nil {
					log.Fatal(err)
				}
			}(conn)
			client := pb.NewSviServiceClient(conn)

			_ = opi.store.Set(testSviName, &testSviWithStatus)
			opi.ListHelper[testSviName] = false
			opi.Pagination["existing-pagination-token"] = 1

			request := &pb.ListSvisRequest{PageSize: tt.size, PageToken: tt.token}
			response, err := client.ListSvis(ctx, request)
			if !utils.EqualProtoSlices(response.GetSvis(), tt.out) {
				t.Error("response: expected", tt.out, "received", response.GetSvis())
			}

			// Empty NextPageToken indicates end of results list
			if tt.size != 1 && response.GetNextPageToken() != "" {
				t.Error("Expected end of results, received non-empty next page token", response.GetNextPageToken())
			}

			if er, ok := status.FromError(err); ok {
				if er.Code() != tt.errCode {
					t.Error("error code: expected", tt.errCode, "received", er.Code())
				}
				if er.Message() != tt.errMsg {
					t.Error("error message: expected", tt.errMsg, "received", er.Message())
				}
			} else {
				t.Error("expected grpc error status")
			}
		})
	}
}
