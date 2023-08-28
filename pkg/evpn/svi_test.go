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
	"strings"
	"testing"

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
	}{
		"illegal resource_id": {
			"CapitalLettersNotAllowed",
			&testSvi,
			nil,
			codes.Unknown,
			fmt.Sprintf("user-settable ID must only contain lowercase, numbers and hyphens (%v)", "got: 'C' in position 0"),
			false,
		},
		"already exists": {
			testSviID,
			&testSvi,
			&testSvi,
			codes.OK,
			"",
			true,
		},
		"no required svi field": {
			testSviID,
			nil,
			nil,
			codes.Unknown,
			"missing required field: svi",
			false,
		},
		"no required vrf field": {
			testSviID,
			&pb.Svi{
				Spec: &pb.SviSpec{},
			},
			nil,
			codes.Unknown,
			"missing required field: svi.spec.vrf",
			false,
		},
		"no required bridge field": {
			testSviID,
			&pb.Svi{
				Spec: &pb.SviSpec{
					Vrf: testVrfName,
				},
			},
			nil,
			codes.Unknown,
			"missing required field: svi.spec.logical_bridge",
			false,
		},
		"no required mac field": {
			testSviID,
			&pb.Svi{
				Spec: &pb.SviSpec{
					Vrf:           testVrfName,
					LogicalBridge: testLogicalBridgeName,
				},
			},
			nil,
			codes.Unknown,
			"missing required field: svi.spec.mac_address",
			false,
		},
		"no required gw ip field": {
			testSviID,
			&pb.Svi{
				Spec: &pb.SviSpec{
					Vrf:           testVrfName,
					LogicalBridge: testLogicalBridgeName,
					MacAddress:    []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
				},
			},
			nil,
			codes.Unknown,
			"missing required field: svi.spec.gw_ip_prefix",
			false,
		},
		"failed LinkByName call": {
			testSviID,
			&testSvi,
			nil,
			codes.NotFound,
			"unable to find key br-tenant",
			false,
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			opi := NewServer()
			mockNetlink := mocks.NewNetlink(t)
			opi.nLink = mockNetlink
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
				opi.Svis[testSviName] = &testSvi
			}
			if tt.out != nil {
				tt.out.Name = testSviName
			}

			// TODO: refactor this mocking
			if strings.Contains(name, "LinkByName") {
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(nil, errors.New(tt.errMsg)).Once()
			}
			if tt.out != nil && !tt.exist {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(bridge, nil).Once()
			}

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
		// 	testSviID,
		// 	&emptypb.Empty{},
		// 	codes.OK,
		// 	"",
		// 	false,
		// },
		"valid request with unknown key": {
			"unknown-id",
			nil,
			codes.NotFound,
			fmt.Sprintf("unable to find key %v", resourceIDToFullName("svis", "unknown-id")),
			false,
		},
		"unknown key with missing allowed": {
			"unknown-id",
			&emptypb.Empty{},
			codes.OK,
			"",
			true,
		},
		"malformed name": {
			"-ABC-DEF",
			&emptypb.Empty{},
			codes.Unknown,
			fmt.Sprintf("segment '%s': not a valid DNS name", "-ABC-DEF"),
			false,
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			opi := NewServer()
			mockNetlink := mocks.NewNetlink(t)
			opi.nLink = mockNetlink
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
			opi.Svis[testSviName] = &testSvi

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
		spdk    []string
		errCode codes.Code
		errMsg  string
		start   bool
		exist   bool
	}{
		"invalid fieldmask": {
			&fieldmaskpb.FieldMask{Paths: []string{"*", "author"}},
			&pb.Svi{
				Name: testSviName,
				Spec: spec,
			},
			nil,
			[]string{""},
			codes.Unknown,
			fmt.Sprintf("invalid field path: %s", "'*' must not be used with other paths"),
			false,
			true,
		},
		"valid request with unknown key": {
			nil,
			&pb.Svi{
				Name: resourceIDToFullName("svis", "unknown-id"),
				Spec: spec,
			},
			nil,
			[]string{""},
			codes.NotFound,
			fmt.Sprintf("unable to find key %v", resourceIDToFullName("svis", "unknown-id")),
			false,
			true,
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			opi := NewServer()
			mockNetlink := mocks.NewNetlink(t)
			opi.nLink = mockNetlink
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
				opi.Svis[testSviName] = &testSvi
			}
			if tt.out != nil {
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
		// 	testSviID,
		// 	&pb.Svi{
		// 		Name: testSviName,
		// 		Spec: testSvi.Spec,
		// 	},
		// 	codes.OK,
		// 	"",
		// },
		"valid request with unknown key": {
			"unknown-id",
			nil,
			codes.NotFound,
			fmt.Sprintf("unable to find key %v", "unknown-id"),
		},
		"malformed name": {
			"-ABC-DEF",
			nil,
			codes.Unknown,
			fmt.Sprintf("segment '%s': not a valid DNS name", "-ABC-DEF"),
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			opi := NewServer()
			mockNetlink := mocks.NewNetlink(t)
			opi.nLink = mockNetlink
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

			opi.Svis[testSviID] = &testSvi

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
