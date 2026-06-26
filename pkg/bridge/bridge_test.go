// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package bridge is the main package of the application
package bridge

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
		Status: &pb.LogicalBridgeStatus{
			OperStatus: pb.LBOperStatus_LB_OPER_STATUS_DOWN,
			Components: []*pb.Component{{Name: "dummy", Status: pb.CompStatus_COMP_STATUS_PENDING}},
		},
	}
	testLogicalBridgeWithStatus = pb.LogicalBridge{
		Name: testLogicalBridgeName,
		Spec: testLogicalBridge.Spec,
		Status: &pb.LogicalBridgeStatus{
			OperStatus: pb.LBOperStatus_LB_OPER_STATUS_DOWN,
			Components: []*pb.Component{{Name: "dummy", Status: pb.CompStatus_COMP_STATUS_PENDING}},
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
					VtepIpPrefix: &pc.IPPrefix{
						Addr: &pc.IPAddress{
							Af: pc.IpAf_IP_AF_INET,
							V4OrV6: &pc.IPAddress_V4Addr{
								V4Addr: 0,
							},
						},
					},
				},
				Status: &pb.LogicalBridgeStatus{
					OperStatus: pb.LBOperStatus_LB_OPER_STATUS_DOWN,
					Components: []*pb.Component{{Name: "dummy", Status: pb.CompStatus_COMP_STATUS_PENDING}},
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
		"successful call": {
			id:      testLogicalBridgeID,
			in:      &testLogicalBridge,
			out:     &testLogicalBridgeWithStatus,
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
			client := pb.NewLogicalBridgeServiceClient(env.conn)

			if tt.exist {
				testLogicalBridgeFull := pb.LogicalBridge{
					Name: testLogicalBridgeName,
					Spec: testLogicalBridge.Spec,
				}
				_, _ = env.opi.createLogicalBridge(&testLogicalBridgeFull)
			}
			if tt.out != nil {
				tt.out = utils.ProtoClone(tt.out)
				tt.out.Name = testLogicalBridgeName
			}
			if tt.on != nil {
				tt.on(env.mockNetlink, env.mockFrr, tt.errMsg)
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
		"successful call": {
			in:      testLogicalBridgeID,
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
			client := pb.NewLogicalBridgeServiceClient(env.conn)

			fname1 := resourceIDToFullName(tt.in)

			testLogicalBridgeFull := pb.LogicalBridge{
				Name: testLogicalBridgeName,
				Spec: testLogicalBridge.Spec,
			}
			_, _ = env.opi.createLogicalBridge(&testLogicalBridgeFull)

			if tt.on != nil {
				tt.on(env.mockNetlink, env.mockFrr, tt.errMsg)
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
			ctx := context.Background()
			env := newTestEnv(ctx, t)
			defer env.Close()
			client := pb.NewLogicalBridgeServiceClient(env.conn)

			if tt.exist {
				testLogicalBridgeFull := pb.LogicalBridge{
					Name: testLogicalBridgeName,
					Spec: testLogicalBridge.Spec,
				}
				_, _ = env.opi.createLogicalBridge(&testLogicalBridgeFull)
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
			ctx := context.Background()
			env := newTestEnv(ctx, t)
			defer env.Close()
			client := pb.NewLogicalBridgeServiceClient(env.conn)

			testLogicalBridgeFull := pb.LogicalBridge{
				Name: testLogicalBridgeName,
				Spec: testLogicalBridge.Spec,
			}
			_, _ = env.opi.createLogicalBridge(&testLogicalBridgeFull)

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
			ctx := context.Background()
			env := newTestEnv(ctx, t)
			defer env.Close()
			client := pb.NewLogicalBridgeServiceClient(env.conn)

			testLogicalBridgeFull := pb.LogicalBridge{
				Name: testLogicalBridgeName,
				Spec: testLogicalBridge.Spec,
			}
			_, _ = env.opi.createLogicalBridge(&testLogicalBridgeFull)
			env.opi.Pagination["existing-pagination-token"] = 1

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

func Test_ListLogicalBridges_Pagination(t *testing.T) {
	ctx := context.Background()
	env := newTestEnv(ctx, t)
	defer env.Close()
	client := pb.NewLogicalBridgeServiceClient(env.conn)

	// Create 2 LogicalBridges
	lb1 := &pb.LogicalBridge{
		Name: resourceIDToFullName("bridge-1"),
		Spec: testLogicalBridge.Spec,
	}
	lb2 := &pb.LogicalBridge{
		Name: resourceIDToFullName("bridge-2"),
		Spec: testLogicalBridge.Spec,
	}
	_, err := env.opi.createLogicalBridge(lb1)
	if err != nil {
		t.Fatalf("failed to create lb1: %v", err)
	}
	_, err = env.opi.createLogicalBridge(lb2)
	if err != nil {
		t.Fatalf("failed to create lb2: %v", err)
	}

	// 1. Get first page, PageSize = 1
	req1 := &pb.ListLogicalBridgesRequest{PageSize: 1}
	res1, err := client.ListLogicalBridges(ctx, req1)
	if err != nil {
		t.Fatalf("ListLogicalBridges page 1 failed: %v", err)
	}

	// Assert only 1 bridge is returned, and NextPageToken is NOT empty
	if len(res1.GetLogicalBridges()) != 1 {
		t.Errorf("expected 1 logical bridge in page 1, got %d", len(res1.GetLogicalBridges()))
	}
	if res1.GetNextPageToken() == "" {
		t.Errorf("expected next page token to be non-empty")
	}

	// 2. Get second page, PageSize = 1, PageToken = res1.GetNextPageToken()
	req2 := &pb.ListLogicalBridgesRequest{PageSize: 1, PageToken: res1.GetNextPageToken()}
	res2, err := client.ListLogicalBridges(ctx, req2)
	if err != nil {
		t.Fatalf("ListLogicalBridges page 2 failed: %v", err)
	}

	// Assert only 1 bridge is returned (the second one) and NextPageToken IS empty
	if len(res2.GetLogicalBridges()) != 1 {
		t.Errorf("expected 1 logical bridge in page 2, got %d", len(res2.GetLogicalBridges()))
	}
	if res2.GetNextPageToken() != "" {
		t.Errorf("expected next page token to be empty, got %q", res2.GetNextPageToken())
	}

	// Check that we got all elements uniquely
	allLbs := append(res1.GetLogicalBridges(), res2.GetLogicalBridges()...)
	names := make(map[string]bool)
	for _, v := range allLbs {
		names[v.GetName()] = true
	}
	if len(names) != 2 {
		t.Errorf("expected 2 unique logical bridges, got: %v", names)
	}
}
