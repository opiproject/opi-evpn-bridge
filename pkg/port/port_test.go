// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package port is the main package of the application
package port

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils/mocks"
)

var (
	testBridgePortID   = "opi-port8"
	testBridgePortName = resourceIDToFullName(testBridgePortID)
	testBridgePort     = pb.BridgePort{
		Spec: &pb.BridgePortSpec{
			MacAddress:     []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
			Ptype:          pb.BridgePortType_BRIDGE_PORT_TYPE_TRUNK,
			LogicalBridges: []string{testLogicalBridgeName},
		},
		Status: &pb.BridgePortStatus{
			OperStatus: pb.BPOperStatus_BP_OPER_STATUS_DOWN,
			Components: []*pb.Component{{Name: "dummy", Status: pb.CompStatus_COMP_STATUS_PENDING}},
		},
	}
	testBridgePortWithStatus = pb.BridgePort{
		Name: testBridgePortName,
		Spec: testBridgePort.Spec,
		Status: &pb.BridgePortStatus{
			OperStatus: pb.BPOperStatus_BP_OPER_STATUS_DOWN,
			Components: []*pb.Component{{Name: "dummy", Status: pb.CompStatus_COMP_STATUS_PENDING}},
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
		on      func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string)
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
			out:     &testBridgePortWithStatus,
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
					Ptype:          pb.BridgePortType_BRIDGE_PORT_TYPE_ACCESS,
					LogicalBridges: []string{"Japan", "Australia", "Germany"},
				},
			},
			out:     nil,
			errCode: codes.InvalidArgument,
			errMsg:  fmt.Sprintf("ACCESS type must have single LogicalBridge and not (%d)", 3),
			exist:   false,
			on:      nil,
		},
		"missing bridges": {
			id: testBridgePortID,
			in: &pb.BridgePort{
				Spec: &pb.BridgePortSpec{
					MacAddress:     []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
					Ptype:          pb.BridgePortType_BRIDGE_PORT_TYPE_TRUNK,
					LogicalBridges: []string{"Japan", "Australia", "Germany"},
				},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "the referenced Logical Bridge has not been found",
			exist:   false,
			on:      nil,
		},
		"successful call": {
			id:      testBridgePortID,
			in:      &testBridgePort,
			out:     &testBridgePortWithStatus,
			errCode: codes.OK,
			errMsg:  "",
			exist:   false,
			on:      nil,
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			env := newTestEnv(ctx, t)
			defer env.Close()
			client := pb.NewBridgePortServiceClient(env.conn)

			testLogicalBridgeFull := pb.LogicalBridge{
				Name: testLogicalBridgeName,
				Spec: testLogicalBridge.Spec,
			}
			_, _ = env.lbServer.TestCreateLogicalBridge(&testLogicalBridgeFull)

			if tt.exist {
				testBridgePortFull := pb.BridgePort{
					Name: testBridgePortName,
					Spec: testBridgePort.Spec,
				}
				_, _ = env.opi.createBridgePort(&testBridgePortFull)
			}
			if tt.out != nil {
				tt.out = utils.ProtoClone(tt.out)
				tt.out.Name = testBridgePortName
			}
			if tt.on != nil {
				tt.on(env.mockNetlink, env.mockFrr, tt.errMsg)
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
		"successful call": {
			in:      testBridgePortID,
			out:     &emptypb.Empty{},
			errCode: codes.OK,
			errMsg:  "",
			missing: false,
			on:      nil,
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			env := newTestEnv(ctx, t)
			defer env.Close()
			client := pb.NewBridgePortServiceClient(env.conn)

			fname1 := resourceIDToFullName(tt.in)

			testLogicalBridgeFull := pb.LogicalBridge{
				Name: testLogicalBridgeName,
				Spec: testLogicalBridge.Spec,
			}
			_, _ = env.lbServer.TestCreateLogicalBridge(&testLogicalBridgeFull)
			testBridgePortFull := pb.BridgePort{
				Name: testBridgePortName,
				Spec: testBridgePort.Spec,
			}
			_, _ = env.opi.createBridgePort(&testBridgePortFull)
			if tt.on != nil {
				tt.on(env.mockNetlink, env.mockFrr, tt.errMsg)
			}

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
		Ptype:          pb.BridgePortType_BRIDGE_PORT_TYPE_ACCESS,
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
			ctx := context.Background()
			env := newTestEnv(ctx, t)
			defer env.Close()
			client := pb.NewBridgePortServiceClient(env.conn)

			if tt.exist {
				testLogicalBridgeFull := pb.LogicalBridge{
					Name: testLogicalBridgeName,
					Spec: testLogicalBridge.Spec,
				}
				_, _ = env.lbServer.TestCreateLogicalBridge(&testLogicalBridgeFull)
				testBridgePortFull := pb.BridgePort{
					Name: testBridgePortName,
					Spec: testBridgePort.Spec,
				}
				_, _ = env.opi.createBridgePort(&testBridgePortFull)
			}
			if tt.out != nil {
				tt.out = utils.ProtoClone(tt.out)
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
			ctx := context.Background()
			env := newTestEnv(ctx, t)
			defer env.Close()
			client := pb.NewBridgePortServiceClient(env.conn)

			testLogicalBridgeFull := pb.LogicalBridge{
				Name: testLogicalBridgeName,
				Spec: testLogicalBridge.Spec,
			}
			_, _ = env.lbServer.TestCreateLogicalBridge(&testLogicalBridgeFull)
			testBridgePortFull := pb.BridgePort{
				Name: testBridgePortName,
				Spec: testBridgePort.Spec,
			}
			_, _ = env.opi.createBridgePort(&testBridgePortFull)

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

func Test_ListBridgePorts(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     []*pb.BridgePort
		errCode codes.Code
		errMsg  string
		size    int32
		token   string
	}{
		"example test": {
			in:      "",
			out:     []*pb.BridgePort{&testBridgePortWithStatus},
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
			out:     []*pb.BridgePort{&testBridgePortWithStatus},
			errCode: codes.OK,
			errMsg:  "",
			size:    1000,
			token:   "",
		},
		"pagination normal": {
			in:      "",
			out:     []*pb.BridgePort{&testBridgePortWithStatus},
			errCode: codes.OK,
			errMsg:  "",
			size:    1,
			token:   "",
		},
		"pagination offset": {
			in:      "",
			out:     []*pb.BridgePort{},
			errCode: codes.OK,
			errMsg:  "",
			size:    1,
			token:   "existing-pagination-token",
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			env := newTestEnv(ctx, t)
			defer env.Close()
			client := pb.NewBridgePortServiceClient(env.conn)

			testLogicalBridgeFull := pb.LogicalBridge{
				Name: testLogicalBridgeName,
				Spec: testLogicalBridge.Spec,
			}
			_, _ = env.lbServer.TestCreateLogicalBridge(&testLogicalBridgeFull)

			testBridgePortFull := pb.BridgePort{
				Name: testBridgePortName,
				Spec: testBridgePort.Spec,
			}
			_, _ = env.opi.createBridgePort(&testBridgePortFull)
			env.opi.Pagination["existing-pagination-token"] = 1

			request := &pb.ListBridgePortsRequest{PageSize: tt.size, PageToken: tt.token}
			response, err := client.ListBridgePorts(ctx, request)
			if !utils.EqualProtoSlices(response.GetBridgePorts(), tt.out) {
				t.Error("response: expected", tt.out, "received", response.GetBridgePorts())
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
