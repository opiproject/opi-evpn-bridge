// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package bridge is the main package of the application
package bridge

import (
	"context"
	"encoding/binary"
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
	testLogicalBridgeID   = "opi-bridge9"
	testLogicalBridgeName = resourceIDToFullName(testLogicalBridgeID)
	testLogicalBridge     = pb.LogicalBridge{
		Spec: &pb.LogicalBridgeSpec{
			Vni:    proto.Uint32(11),
			VlanId: 22,
			VtepIpPrefix: &pc.IPPrefix{
				Addr: &pc.IPAddress{
					Af: pc.IpAf_IP_AF_INET,
					V4OrV6: &pc.IPAddress_V4Addr{
						V4Addr: 167772162,
					},
				},
				Len: 24,
			},
		},
	}
	testLogicalBridgeWithStatus = pb.LogicalBridge{
		Name: testLogicalBridgeName,
		Spec: testLogicalBridge.Spec,
		Status: &pb.LogicalBridgeStatus{
			OperStatus: pb.LBOperStatus_LB_OPER_STATUS_UP,
		},
	}
)

func Test_CreateLogicalBridge(t *testing.T) {
	tests := map[string]struct {
		id      string
		in      *pb.LogicalBridge
		out     *pb.LogicalBridge
		errCode codes.Code
		errMsg  string
		exist   bool
		on      func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string)
	}{
		"illegal resource_id": {
			id:      "CapitalLettersNotAllowed",
			in:      &testLogicalBridge,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  fmt.Sprintf("user-settable ID must only contain lowercase, numbers and hyphens (%v)", "got: 'C' in position 0"),
			exist:   false,
			on:      nil,
		},
		"no required bridge field": {
			id:      testLogicalBridgeID,
			in:      nil,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: logical_bridge",
			exist:   false,
			on:      nil,
		},
		"no required vlan_id field": {
			id: testLogicalBridgeID,
			in: &pb.LogicalBridge{
				Spec: &pb.LogicalBridgeSpec{},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: logical_bridge.spec.vlan_id",
			exist:   false,
			on:      nil,
		},
		"illegal VlanId": {
			id: testLogicalBridgeID,
			in: &pb.LogicalBridge{
				Spec: &pb.LogicalBridgeSpec{
					Vni:    proto.Uint32(11),
					VlanId: 4096,
				},
			},
			out:     nil,
			errCode: codes.InvalidArgument,
			errMsg:  fmt.Sprintf("VlanId value (%v) have to be between 1 and 4095", 4096),
			exist:   false,
			on:      nil,
		},
		"empty vni": {
			id: testLogicalBridgeID,
			in: &pb.LogicalBridge{
				Spec: &pb.LogicalBridgeSpec{
					VlanId: 11,
				},
			},
			out: &pb.LogicalBridge{
				Spec: &pb.LogicalBridgeSpec{
					VlanId: 11,
				},
				Status: &pb.LogicalBridgeStatus{
					OperStatus: pb.LBOperStatus_LB_OPER_STATUS_UP,
				},
			},
			errCode: codes.OK,
			errMsg:  "",
			exist:   false,
			on:      nil,
		},
		"already exists": {
			id:      testLogicalBridgeID,
			in:      &testLogicalBridge,
			out:     &testLogicalBridgeWithStatus,
			errCode: codes.OK,
			errMsg:  "",
			exist:   true,
		},
		"failed LinkByName call": {
			id:      testLogicalBridgeID,
			in:      &testLogicalBridge,
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", tenantbridgeName),
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed LinkAdd call": {
			id:      testLogicalBridgeID,
			in:      &testLogicalBridge,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkAdd",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				// myip := net.ParseIP("10.0.0.2")
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testLogicalBridge.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testLogicalBridge.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vxlan).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetMaster call": {
			id:      testLogicalBridgeID,
			in:      &testLogicalBridge,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetMaster",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testLogicalBridge.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testLogicalBridge.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(mock.Anything, vxlan, bridge).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetUp call": {
			id:      testLogicalBridgeID,
			in:      &testLogicalBridge,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetUp",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testLogicalBridge.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testLogicalBridge.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(mock.Anything, vxlan, bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(mock.Anything, vxlan).Return(errors.New(errMsg)).Once()
			},
		},
		"failed BridgeVlanAdd call": {
			id:      testLogicalBridgeID,
			in:      &testLogicalBridge,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call BridgeVlanAdd",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testLogicalBridge.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testLogicalBridge.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(mock.Anything, vxlan, bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(mock.Anything, vxlan).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, vxlan, vid, true, true, false, false).Return(errors.New(errMsg)).Once()
			},
		},
		"successful call": {
			id:      testLogicalBridgeID,
			in:      &testLogicalBridge,
			out:     &testLogicalBridgeWithStatus,
			errCode: codes.OK,
			errMsg:  "",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testLogicalBridge.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testLogicalBridge.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything, tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkAdd(mock.Anything, vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(mock.Anything, vxlan, bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(mock.Anything, vxlan).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, vxlan, vid, true, true, false, false).Return(nil).Once()
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
			client := pb.NewLogicalBridgeServiceClient(conn)

			if tt.exist {
				_ = opi.store.Set(testLogicalBridgeName, &testLogicalBridgeWithStatus)
			}
			if tt.out != nil {
				tt.out = utils.ProtoClone(tt.out)
				tt.out.Name = testLogicalBridgeName
			}
			if tt.on != nil {
				tt.on(mockNetlink, mockFrr, tt.errMsg)
			}

			request := &pb.CreateLogicalBridgeRequest{LogicalBridge: tt.in, LogicalBridgeId: tt.id}
			response, err := client.CreateLogicalBridge(ctx, request)
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

func Test_DeleteLogicalBridge(t *testing.T) {
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
		"failed LinkByName call": {
			in:      testLogicalBridgeID,
			out:     &emptypb.Empty{},
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", "vni11"),
			missing: false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				vxlanName := fmt.Sprintf("vni%d", *testLogicalBridge.Spec.Vni)
				mockNetlink.EXPECT().LinkByName(mock.Anything, vxlanName).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetDown call": {
			in:      testLogicalBridgeID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetDown",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testLogicalBridge.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testLogicalBridge.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(mock.Anything, vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(mock.Anything, vxlan).Return(errors.New(errMsg)).Once()
			},
		},
		"failed BridgeVlanDel call": {
			in:      testLogicalBridgeID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call BridgeVlanDel",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testLogicalBridge.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testLogicalBridge.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(mock.Anything, vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(mock.Anything, vxlan).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanDel(mock.Anything, vxlan, vid, true, true, false, false).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkDel call": {
			in:      testLogicalBridgeID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkDel",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testLogicalBridge.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testLogicalBridge.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(mock.Anything, vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(mock.Anything, vxlan).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanDel(mock.Anything, vxlan, vid, true, true, false, false).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(mock.Anything, vxlan).Return(errors.New(errMsg)).Once()
			},
		},
		"successful call": {
			in:      testLogicalBridgeID,
			out:     &emptypb.Empty{},
			errCode: codes.OK,
			errMsg:  "",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testLogicalBridge.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testLogicalBridge.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(mock.Anything, vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(mock.Anything, vxlan).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanDel(mock.Anything, vxlan, vid, true, true, false, false).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(mock.Anything, vxlan).Return(nil).Once()
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
			client := pb.NewLogicalBridgeServiceClient(conn)

			fname1 := resourceIDToFullName(tt.in)
			_ = opi.store.Set(testLogicalBridgeName, &testLogicalBridgeWithStatus)

			if tt.on != nil {
				tt.on(mockNetlink, mockFrr, tt.errMsg)
			}

			request := &pb.DeleteLogicalBridgeRequest{Name: fname1, AllowMissing: tt.missing}
			response, err := client.DeleteLogicalBridge(ctx, request)

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

func Test_UpdateLogicalBridge(t *testing.T) {
	spec := &pb.LogicalBridgeSpec{
		Vni:    proto.Uint32(11),
		VlanId: 22,
	}
	tests := map[string]struct {
		mask    *fieldmaskpb.FieldMask
		in      *pb.LogicalBridge
		out     *pb.LogicalBridge
		errCode codes.Code
		errMsg  string
		start   bool
		exist   bool
	}{
		"invalid fieldmask": {
			mask: &fieldmaskpb.FieldMask{Paths: []string{"*", "author"}},
			in: &pb.LogicalBridge{
				Name: testLogicalBridgeName,
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
			in: &pb.LogicalBridge{
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
			client := pb.NewLogicalBridgeServiceClient(conn)

			if tt.exist {
				_ = opi.store.Set(testLogicalBridgeName, &testLogicalBridgeWithStatus)
			}
			if tt.out != nil {
				tt.out = utils.ProtoClone(tt.out)
				tt.out.Name = testLogicalBridgeName
			}

			request := &pb.UpdateLogicalBridgeRequest{LogicalBridge: tt.in, UpdateMask: tt.mask}
			response, err := client.UpdateLogicalBridge(ctx, request)
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

func Test_GetLogicalBridge(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     *pb.LogicalBridge
		errCode codes.Code
		errMsg  string
	}{
		// "valid request": {
		// 	in: testLogicalBridgeID,
		// 	out: &pb.LogicalBridge{
		// 		Name:      testLogicalBridgeName,
		// 		Multipath: testLogicalBridge.Multipath,
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
			client := pb.NewLogicalBridgeServiceClient(conn)

			_ = opi.store.Set(testLogicalBridgeName, &testLogicalBridgeWithStatus)

			request := &pb.GetLogicalBridgeRequest{Name: tt.in}
			response, err := client.GetLogicalBridge(ctx, request)
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

func Test_ListLogicalBridges(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     []*pb.LogicalBridge
		errCode codes.Code
		errMsg  string
		size    int32
		token   string
	}{
		"example test": {
			in:      "",
			out:     []*pb.LogicalBridge{&testLogicalBridgeWithStatus},
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
			out:     []*pb.LogicalBridge{&testLogicalBridgeWithStatus},
			errCode: codes.OK,
			errMsg:  "",
			size:    1000,
			token:   "",
		},
		"pagination normal": {
			in:      "",
			out:     []*pb.LogicalBridge{&testLogicalBridgeWithStatus},
			errCode: codes.OK,
			errMsg:  "",
			size:    1,
			token:   "",
		},
		"pagination offset": {
			in:      "",
			out:     []*pb.LogicalBridge{},
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
			client := pb.NewLogicalBridgeServiceClient(conn)

			_ = opi.store.Set(testLogicalBridgeName, &testLogicalBridgeWithStatus)
			opi.ListHelper[testLogicalBridgeName] = false
			opi.Pagination["existing-pagination-token"] = 1

			request := &pb.ListLogicalBridgesRequest{PageSize: tt.size, PageToken: tt.token}
			response, err := client.ListLogicalBridges(ctx, request)
			if !utils.EqualProtoSlices(response.GetLogicalBridges(), tt.out) {
				t.Error("response: expected", tt.out, "received", response.GetLogicalBridges())
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
