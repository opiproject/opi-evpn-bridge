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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"github.com/opiproject/opi-evpn-bridge/pkg/utils/mocks"
)

var (
	testBridgePortID   = "opi-port8"
	testBridgePortName = resourceIDToFullName("ports", testBridgePortID)
	testBridgePort     = pb.BridgePort{
		Spec: &pb.BridgePortSpec{
			MacAddress:     []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
			Ptype:          pb.BridgePortType_TRUNK,
			LogicalBridges: []string{"Japan", "Australia", "Germany"},
		},
	}
)

func Test_CreateBridgePort(t *testing.T) {
	tests := map[string]struct {
		id      string
		in      *pb.BridgePort
		out     *pb.BridgePort
		errCode codes.Code
		errMsg  string
		exist   bool
	}{
		"illegal resource_id": {
			"CapitalLettersNotAllowed",
			&testBridgePort,
			nil,
			codes.Unknown,
			fmt.Sprintf("user-settable ID must only contain lowercase, numbers and hyphens (%v)", "got: 'C' in position 0"),
			false,
		},
		"already exists": {
			testBridgePortID,
			&testBridgePort,
			&testBridgePort,
			codes.OK,
			"",
			true,
		},
		"no required port field": {
			testBridgePortID,
			nil,
			nil,
			codes.Unknown,
			"missing required field: bridge_port",
			false,
		},
		"no required mac_address field": {
			testBridgePortID,
			&pb.BridgePort{
				Spec: &pb.BridgePortSpec{},
			},
			nil,
			codes.Unknown,
			"missing required field: bridge_port.spec.mac_address",
			false,
		},
		"no required ptype field": {
			testBridgePortID,
			&pb.BridgePort{
				Spec: &pb.BridgePortSpec{
					MacAddress: []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
				},
			},
			nil,
			codes.Unknown,
			"missing required field: bridge_port.spec.ptype",
			false,
		},
		"access port and list bridge": {
			testBridgePortID,
			&pb.BridgePort{
				Spec: &pb.BridgePortSpec{
					MacAddress:     []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
					Ptype:          pb.BridgePortType_ACCESS,
					LogicalBridges: []string{"Japan", "Australia", "Germany"},
				},
			},
			nil,
			codes.InvalidArgument,
			fmt.Sprintf("ACCESS type must have single LogicalBridge and not (%d)", len(testBridgePort.Spec.LogicalBridges)),
			false,
		},
		"failed LinkByName call": {
			testBridgePortID,
			&testBridgePort,
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
			client := pb.NewBridgePortServiceClient(conn)

			if tt.exist {
				opi.Ports[testBridgePortName] = protoClone(&testBridgePort)
				opi.Ports[testBridgePortName].Name = testBridgePortName
			}
			if tt.out != nil {
				tt.out = protoClone(tt.out)
				tt.out.Name = testBridgePortName
			}

			// TODO: refactor this mocking
			if strings.Contains(name, "LinkByName") {
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(nil, errors.New(tt.errMsg)).Once()
			}

			request := &pb.CreateBridgePortRequest{BridgePort: tt.in, BridgePortId: tt.id}
			response, err := client.CreateBridgePort(ctx, request)
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

func Test_DeleteBridgePort(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     *emptypb.Empty
		errCode codes.Code
		errMsg  string
		missing bool
	}{
		// "valid request": {
		// 	testBridgePortID,
		// 	&emptypb.Empty{},
		// 	codes.OK,
		// 	"",
		// 	false,
		// },
		"valid request with unknown key": {
			"unknown-id",
			nil,
			codes.NotFound,
			fmt.Sprintf("unable to find key %v", resourceIDToFullName("ports", "unknown-id")),
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
			client := pb.NewBridgePortServiceClient(conn)

			fname1 := resourceIDToFullName("ports", tt.in)
			opi.Ports[testBridgePortName] = protoClone(&testBridgePort)

			request := &pb.DeleteBridgePortRequest{Name: fname1, AllowMissing: tt.missing}
			response, err := client.DeleteBridgePort(ctx, request)

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

func Test_UpdateBridgePort(t *testing.T) {
	spec := &pb.BridgePortSpec{
		MacAddress:     []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
		Ptype:          pb.BridgePortType_ACCESS,
		LogicalBridges: []string{"Japan", "Australia", "Germany"},
	}
	tests := map[string]struct {
		mask    *fieldmaskpb.FieldMask
		in      *pb.BridgePort
		out     *pb.BridgePort
		spdk    []string
		errCode codes.Code
		errMsg  string
		start   bool
		exist   bool
	}{
		"invalid fieldmask": {
			&fieldmaskpb.FieldMask{Paths: []string{"*", "author"}},
			&pb.BridgePort{
				Name: testBridgePortName,
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
			&pb.BridgePort{
				Name: resourceIDToFullName("ports", "unknown-id"),
				Spec: spec,
			},
			nil,
			[]string{""},
			codes.NotFound,
			fmt.Sprintf("unable to find key %v", resourceIDToFullName("ports", "unknown-id")),
			false,
			true,
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
			client := pb.NewBridgePortServiceClient(conn)

			if tt.exist {
				opi.Ports[testBridgePortName] = protoClone(&testBridgePort)
				opi.Ports[testBridgePortName].Name = testBridgePortName
			}
			if tt.out != nil {
				tt.out = protoClone(tt.out)
				tt.out.Name = testBridgePortName
			}

			request := &pb.UpdateBridgePortRequest{BridgePort: tt.in, UpdateMask: tt.mask}
			response, err := client.UpdateBridgePort(ctx, request)
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

func Test_GetBridgePort(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     *pb.BridgePort
		errCode codes.Code
		errMsg  string
	}{
		// "valid request": {
		// 	testBridgePortID,
		// 	&pb.BridgePort{
		// 		Name:      testBridgePortName,
		// 		Multipath: testBridgePort.Multipath,
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
			client := pb.NewBridgePortServiceClient(conn)

			opi.Ports[testBridgePortID] = protoClone(&testBridgePort)

			request := &pb.GetBridgePortRequest{Name: tt.in}
			response, err := client.GetBridgePort(ctx, request)
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
