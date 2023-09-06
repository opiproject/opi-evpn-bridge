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
		on      func(mockNetlink *mocks.Netlink, errMsg string)
	}{
		"illegal resource_id": {
			id:      "CapitalLettersNotAllowed",
			in:      &testBridgePort,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  fmt.Sprintf("user-settable ID must only contain lowercase, numbers and hyphens (%v)", "got: 'C' in position 0"),
			exist:   false,
			on:      nil,
		},
		"already exists": {
			id:      testBridgePortID,
			in:      &testBridgePort,
			out:     &testBridgePort,
			errCode: codes.OK,
			errMsg:  "",
			exist:   true,
			on:      nil,
		},
		"no required port field": {
			id:      testBridgePortID,
			in:      nil,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: bridge_port",
			exist:   false,
			on:      nil,
		},
		"no required mac_address field": {
			id: testBridgePortID,
			in: &pb.BridgePort{
				Spec: &pb.BridgePortSpec{},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: bridge_port.spec.mac_address",
			exist:   false,
			on:      nil,
		},
		"no required ptype field": {
			id: testBridgePortID,
			in: &pb.BridgePort{
				Spec: &pb.BridgePortSpec{
					MacAddress: []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
				},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: bridge_port.spec.ptype",
			exist:   false,
			on:      nil,
		},
		"access port and list bridge": {
			id: testBridgePortID,
			in: &pb.BridgePort{
				Spec: &pb.BridgePortSpec{
					MacAddress:     []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
					Ptype:          pb.BridgePortType_ACCESS,
					LogicalBridges: []string{"Japan", "Australia", "Germany"},
				},
			},
			out:     nil,
			errCode: codes.InvalidArgument,
			errMsg:  fmt.Sprintf("ACCESS type must have single LogicalBridge and not (%d)", len(testBridgePort.Spec.LogicalBridges)),
			exist:   false,
			on:      nil,
		},
		"failed LinkByName call": {
			id:      testBridgePortID,
			in:      &testBridgePort,
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  "unable to find key br-tenant",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				mockNetlink.EXPECT().LinkByName(mock.Anything).Return(nil, errors.New(errMsg)).Once()
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
			client := pb.NewBridgePortServiceClient(conn)

			if tt.exist {
				opi.Ports[testBridgePortName] = protoClone(&testBridgePort)
				opi.Ports[testBridgePortName].Name = testBridgePortName
			}
			if tt.out != nil {
				tt.out = protoClone(tt.out)
				tt.out.Name = testBridgePortName
			}
			if tt.on != nil {
				tt.on(mockNetlink, tt.errMsg)
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
		// 	in: testBridgePortID,
		// 	out: &emptypb.Empty{},
		// 	errCode: codes.OK,
		// 	errMsg: "",
		// 	missing: false,
		// },
		"valid request with unknown key": {
			in:      "unknown-id",
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", resourceIDToFullName("ports", "unknown-id")),
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
		errCode codes.Code
		errMsg  string
		start   bool
		exist   bool
	}{
		"invalid fieldmask": {
			mask: &fieldmaskpb.FieldMask{Paths: []string{"*", "author"}},
			in: &pb.BridgePort{
				Name: testBridgePortName,
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
			in: &pb.BridgePort{
				Name: resourceIDToFullName("ports", "unknown-id"),
				Spec: spec,
			},
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", resourceIDToFullName("ports", "unknown-id")),
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
		// 	in: testBridgePortID,
		// 	out: &pb.BridgePort{
		// 		Name:      testBridgePortName,
		// 		Multipath: testBridgePort.Multipath,
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
