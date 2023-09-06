// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"testing"

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

	"github.com/opiproject/opi-evpn-bridge/pkg/utils/mocks"
)

var (
	testSviID   = "opi-svi8"
	testSviName = resourceIDToFullName("svis", testSviID)
	testSvi     = pb.Svi{
		Spec: &pb.SviSpec{
			Vrf:           testVrfName,
			LogicalBridge: testLogicalBridgeName,
			MacAddress:    []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
			GwIpPrefix:    []*pc.IPPrefix{{Len: 24}},
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
		on      func(mockNetlink *mocks.Netlink, errMsg string)
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
			out:     &testSvi,
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
		"failed LinkByName call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  "unable to find key br-tenant",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				mockNetlink.EXPECT().LinkByName(mock.Anything).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed BridgeVlanAdd call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call BridgeVlanAdd",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything).Return(bridge, nil).Once()
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkAdd call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkAdd",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything).Return(bridge, nil).Once()
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
				mockNetlink.EXPECT().LinkAdd(mock.Anything).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetHardwareAddr call": {
			id:      testSviID,
			in:      &testSvi,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetHardwareAddr",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(mock.Anything).Return(bridge, nil).Once()
				mockNetlink.EXPECT().BridgeVlanAdd(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
				mockNetlink.EXPECT().LinkAdd(mock.Anything).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetHardwareAddr(mock.Anything, mock.Anything).Return(errors.New(errMsg)).Once()
			},
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			mockNetlink := mocks.NewNetlink(t)
			opi := NewServerWithArgs(mockNetlink)
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
				opi.Svis[testSviName] = protoClone(&testSvi)
				opi.Svis[testSviName].Name = testSviName
			}
			if tt.out != nil {
				tt.out = protoClone(tt.out)
				tt.out.Name = testSviName
			}
			if tt.on != nil {
				tt.on(mockNetlink, tt.errMsg)
			}
			opi.Vrfs[testVrfName] = protoClone(&testVrf)
			opi.Bridges[testLogicalBridgeName] = protoClone(&testLogicalBridge)

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
	}{
		// "valid request": {
		// 	in: testSviID,
		// 	out: &emptypb.Empty{},
		// 	errCode: codes.OK,
		// 	errMsg: "",
		// 	missing: false,
		// },
		"valid request with unknown key": {
			in:      "unknown-id",
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", resourceIDToFullName("svis", "unknown-id")),
			missing: false,
		},
		"unknown key with missing allowed": {
			in:      "unknown-id",
			out:     &emptypb.Empty{},
			errCode: codes.OK,
			errMsg:  "",
			missing: true,
		},
		"malformed name": {
			in:      "-ABC-DEF",
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  fmt.Sprintf("segment '%s': not a valid DNS name", "-ABC-DEF"),
			missing: false,
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			mockNetlink := mocks.NewNetlink(t)
			opi := NewServerWithArgs(mockNetlink)
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

			fname1 := resourceIDToFullName("svis", tt.in)
			opi.Svis[testSviName] = protoClone(&testSvi)

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
				Name: resourceIDToFullName("svis", "unknown-id"),
				Spec: spec,
			},
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", resourceIDToFullName("svis", "unknown-id")),
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
			opi := NewServerWithArgs(mockNetlink)
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
				opi.Svis[testSviName] = protoClone(&testSvi)
				opi.Svis[testSviName].Name = testSviName
			}
			if tt.out != nil {
				tt.out = protoClone(tt.out)
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
			opi := NewServerWithArgs(mockNetlink)
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

			opi.Svis[testSviName] = protoClone(&testSvi)

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
