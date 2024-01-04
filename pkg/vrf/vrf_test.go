// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package vrf is the main package of the application
package vrf

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
	testVrfID   = "opi-vrf8"
	testVrfName = resourceIDToFullName(testVrfID)
	testVrf     = pb.Vrf{
		Spec: &pb.VrfSpec{
			Vni: proto.Uint32(1000),
			LoopbackIpPrefix: &pc.IPPrefix{
				Addr: &pc.IPAddress{
					Af: pc.IpAf_IP_AF_INET,
					V4OrV6: &pc.IPAddress_V4Addr{
						V4Addr: 167772162,
					},
				},
				Len: 24,
			},
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
	testVrfWithStatus = pb.Vrf{
		Name: testVrfName,
		Spec: testVrf.Spec,
		Status: &pb.VrfStatus{
			OperStatus: pb.VRFOperStatus_VRF_OPER_STATUS_DOWN,
		},
	}
)

func Test_CreateVrf(t *testing.T) {
	tests := map[string]struct {
		id      string
		in      *pb.Vrf
		out     *pb.Vrf
		errCode codes.Code
		errMsg  string
		exist   bool
		on      func(mockNetlink *mocks.Netlink, mockFrr *mocks.Frr, errMsg string)
	}{
		"illegal resource_id": {
			id:      "CapitalLettersNotAllowed",
			in:      &testVrf,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  fmt.Sprintf("user-settable ID must only contain lowercase, numbers and hyphens (%v)", "got: 'C' in position 0"),
			exist:   false,
			on:      nil,
		},
		"no required vrf field": {
			id:      testVrfID,
			in:      nil,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: vrf",
			exist:   false,
			on:      nil,
		},
		"no required loopback_ip_prefix field": {
			id: testVrfID,
			in: &pb.Vrf{
				Spec: &pb.VrfSpec{},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "missing required field: vrf.spec.loopback_ip_prefix",
			exist:   false,
			on:      nil,
		},
		"already exists": {
			id:      testVrfID,
			in:      &testVrf,
			out:     &testVrfWithStatus,
			errCode: codes.OK,
			errMsg:  "",
			exist:   true,
			on:      nil,
		},
		"valid request empty VNI": {
			id: testVrfID,
			in: &pb.Vrf{
				Spec: &pb.VrfSpec{
					LoopbackIpPrefix: &pc.IPPrefix{
						Addr: &pc.IPAddress{
							Af: pc.IpAf_IP_AF_INET,
							V4OrV6: &pc.IPAddress_V4Addr{
								V4Addr: 167772162,
							},
						},
						Len: 24,
					},
				},
			},
			out: &pb.Vrf{
				Spec: &pb.VrfSpec{
					LoopbackIpPrefix: &pc.IPPrefix{
						Addr: &pc.IPAddress{
							Af: pc.IpAf_IP_AF_INET,
							V4OrV6: &pc.IPAddress_V4Addr{
								V4Addr: 167772162,
							},
						},
						Len: 24,
					},
				},
				Status: &pb.VrfStatus{
					OperStatus: pb.VRFOperStatus_VRF_OPER_STATUS_DOWN,
				},
			},
			errCode: codes.OK,
			errMsg:  "",
			exist:   false,
			on:      nil,
		},
		"successful call": {
			id:      testVrfID,
			in:      &testVrf,
			out:     &testVrfWithStatus,
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
			client := pb.NewVrfServiceClient(env.conn)

			if tt.exist {
				testVrfFull := pb.Vrf{
					Name: testVrfName,
					Spec: testVrf.Spec,
				}
				_, _ = env.opi.createVrf(&testVrfFull)
			}
			if tt.out != nil {
				tt.out = utils.ProtoClone(tt.out)
				tt.out.Name = testVrfName
			}
			if tt.on != nil {
				tt.on(env.mockNetlink, env.mockFrr, tt.errMsg)
			}

			request := &pb.CreateVrfRequest{Vrf: tt.in, VrfId: tt.id}
			response, err := client.CreateVrf(ctx, request)
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

func Test_DeleteVrf(t *testing.T) {
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
			in:      testVrfID,
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
			client := pb.NewVrfServiceClient(env.conn)

			fname1 := resourceIDToFullName(tt.in)
			testVrfFull := pb.Vrf{
				Name: testVrfName,
				Spec: testVrf.Spec,
			}
			_, _ = env.opi.createVrf(&testVrfFull)
			if tt.on != nil {
				tt.on(env.mockNetlink, env.mockFrr, tt.errMsg)
			}

			request := &pb.DeleteVrfRequest{Name: fname1, AllowMissing: tt.missing}
			response, err := client.DeleteVrf(ctx, request)

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

func Test_UpdateVrf(t *testing.T) {
	spec := &pb.VrfSpec{
		Vni: proto.Uint32(1000),
		LoopbackIpPrefix: &pc.IPPrefix{
			Addr: &pc.IPAddress{
				Af: pc.IpAf_IP_AF_INET,
				V4OrV6: &pc.IPAddress_V4Addr{
					V4Addr: 336860161,
				},
			},
			Len: 24,
		},
	}
	tests := map[string]struct {
		mask    *fieldmaskpb.FieldMask
		in      *pb.Vrf
		out     *pb.Vrf
		errCode codes.Code
		errMsg  string
		start   bool
		exist   bool
	}{
		"invalid fieldmask": {
			mask: &fieldmaskpb.FieldMask{Paths: []string{"*", "author"}},
			in: &pb.Vrf{
				Name: testVrfName,
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
			in: &pb.Vrf{
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
			client := pb.NewVrfServiceClient(env.conn)

			if tt.exist {
				testVrfFull := pb.Vrf{
					Name: testVrfName,
					Spec: testVrf.Spec,
				}
				_, _ = env.opi.createVrf(&testVrfFull)
			}
			if tt.out != nil {
				tt.out = utils.ProtoClone(tt.out)
				tt.out.Name = testVrfName
			}

			request := &pb.UpdateVrfRequest{Vrf: tt.in, UpdateMask: tt.mask}
			response, err := client.UpdateVrf(ctx, request)
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

func Test_GetVrf(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     *pb.Vrf
		errCode codes.Code
		errMsg  string
	}{
		// "valid request": {
		// 	in: testVrfID,
		// 	out: &pb.Vrf{
		// 		Name:      testVrfName,
		// 		Multipath: testVrf.Multipath,
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
			client := pb.NewVrfServiceClient(env.conn)

			testVrfFull := pb.Vrf{
				Name: testVrfName,
				Spec: testVrf.Spec,
			}
			_, _ = env.opi.createVrf(&testVrfFull)

			request := &pb.GetVrfRequest{Name: tt.in}
			response, err := client.GetVrf(ctx, request)
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

func Test_ListVrfs(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     []*pb.Vrf
		errCode codes.Code
		errMsg  string
		size    int32
		token   string
	}{
		"example test": {
			in:      "",
			out:     []*pb.Vrf{&testVrfWithStatus},
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
			out:     []*pb.Vrf{&testVrfWithStatus},
			errCode: codes.OK,
			errMsg:  "",
			size:    1000,
			token:   "",
		},
		"pagination normal": {
			in:      "",
			out:     []*pb.Vrf{&testVrfWithStatus},
			errCode: codes.OK,
			errMsg:  "",
			size:    1,
			token:   "",
		},
		"pagination offset": {
			in:      "",
			out:     []*pb.Vrf{},
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
			client := pb.NewVrfServiceClient(env.conn)

			testVrfFull := pb.Vrf{
				Name: testVrfName,
				Spec: testVrf.Spec,
			}
			_, _ = env.opi.createVrf(&testVrfFull)
			env.opi.Pagination["existing-pagination-token"] = 1

			request := &pb.ListVrfsRequest{PageSize: tt.size, PageToken: tt.token}
			response, err := client.ListVrfs(ctx, request)
			if !utils.EqualProtoSlices(response.GetVrfs(), tt.out) {
				t.Error("response: expected", tt.out, "received", response.GetVrfs())
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
