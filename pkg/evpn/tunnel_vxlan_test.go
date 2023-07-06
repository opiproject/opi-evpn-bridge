// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"reflect"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "github.com/opiproject/opi-api/network/cloud/v1alpha1/gen/go"
	pc "github.com/opiproject/opi-api/network/opinetcommon/v1alpha1/gen/go"
)

var (
	testTunnelID   = "opi-tunnel8"
	testTunnelName = resourceIDToVolumeName("tunnels", testTunnelID)
	testTunnel     = pb.Tunnel{
		Spec: &pb.TunnelSpec{
			LocalIp: &pc.IPAddress{
				Af:     pc.IpAf_IP_AF_INET,
				V4OrV6: &pc.IPAddress_V4Addr{V4Addr: 336860161},
			},
			Encap: &pc.Encap{
				Type: pc.EncapType_ENCAP_TYPE_VXLAN,
				Value: &pc.EncapVal{
					Val: &pc.EncapVal_Vnid{Vnid: 100},
				},
			},
		},
	}
)

func Test_CreateTunnel(t *testing.T) {
	tests := map[string]struct {
		id      string
		in      *pb.Tunnel
		out     *pb.Tunnel
		errCode codes.Code
		errMsg  string
		exist   bool
	}{
		"illegal resource_id": {
			"CapitalLettersNotAllowed",
			&testTunnel,
			nil,
			codes.Unknown,
			fmt.Sprintf("user-settable ID must only contain lowercase, numbers and hyphens (%v)", "got: 'C' in position 0"),
			false,
		},
		"already exists": {
			testTunnelID,
			&testTunnel,
			&testTunnel,
			codes.OK,
			"",
			true,
		},
	}

	// run tests
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// start GRPC mockup server
			ctx := context.Background()
			opi := NewServer()
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
			client := pb.NewCloudInfraServiceClient(conn)

			if tt.exist {
				opi.Tunnels[testTunnelName] = &testTunnel
			}
			if tt.out != nil {
				tt.out.Name = testTunnelName
			}

			request := &pb.CreateTunnelRequest{Tunnel: tt.in, TunnelId: tt.id, Parent: "todo"}
			response, err := client.CreateTunnel(ctx, request)
			if response != nil {
				// if !reflect.DeepEqual(response, tt.out) {
				mtt, _ := proto.Marshal(tt.out)
				mResponse, _ := proto.Marshal(response)
				if !bytes.Equal(mtt, mResponse) {
					t.Error("response: expected", tt.out, "received", response)
				}
			}

			if err != nil {
				if er, ok := status.FromError(err); ok {
					if er.Code() != tt.errCode {
						t.Error("error code: expected", tt.errCode, "received", er.Code())
					}
					if er.Message() != tt.errMsg {
						t.Error("error message: expected", tt.errMsg, "received", er.Message())
					}
				}
			}
		})
	}
}

func Test_DeleteTunnel(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     *emptypb.Empty
		errCode codes.Code
		errMsg  string
		missing bool
	}{
		// "valid request": {
		// 	testTunnelID,
		// 	&emptypb.Empty{},
		// 	codes.OK,
		// 	"",
		// 	false,
		// },
		"valid request with unknown key": {
			"unknown-id",
			nil,
			codes.NotFound,
			fmt.Sprintf("unable to find key %v", resourceIDToVolumeName("tunnels", "unknown-id")),
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
			client := pb.NewCloudInfraServiceClient(conn)

			fname1 := resourceIDToVolumeName("tunnels", tt.in)
			opi.Tunnels[testTunnelName] = &testTunnel

			request := &pb.DeleteTunnelRequest{Name: fname1, AllowMissing: tt.missing}
			response, err := client.DeleteTunnel(ctx, request)
			if err != nil {
				if er, ok := status.FromError(err); ok {
					if er.Code() != tt.errCode {
						t.Error("error code: expected", tt.errCode, "received", er.Code())
					}
					if er.Message() != tt.errMsg {
						t.Error("error message: expected", tt.errMsg, "received", er.Message())
					}
				}
			}
			if reflect.TypeOf(response) != reflect.TypeOf(tt.out) {
				t.Error("response: expected", reflect.TypeOf(tt.out), "received", reflect.TypeOf(response))
			}
		})
	}
}

func Test_GetTunnel(t *testing.T) {
	tests := map[string]struct {
		in      string
		out     *pb.Tunnel
		errCode codes.Code
		errMsg  string
	}{
		// "valid request": {
		// 	testTunnelID,
		// 	&pb.Tunnel{
		// 		Name:      testTunnelName,
		// 		Multipath: testTunnel.Multipath,
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
			client := pb.NewCloudInfraServiceClient(conn)

			opi.Tunnels[testTunnelID] = &testTunnel

			request := &pb.GetTunnelRequest{Name: tt.in}
			response, err := client.GetTunnel(ctx, request)
			if response != nil {
				// if !reflect.DeepEqual(response, tt.out) {
				mtt, _ := proto.Marshal(tt.out)
				mResponse, _ := proto.Marshal(response)
				if !bytes.Equal(mtt, mResponse) {
					t.Error("response: expected", tt.out, "received", response)
				}
			}

			if err != nil {
				if er, ok := status.FromError(err); ok {
					if er.Code() != tt.errCode {
						t.Error("error code: expected", tt.errCode, "received", er.Code())
					}
					if er.Message() != tt.errMsg {
						t.Error("error message: expected", tt.errMsg, "received", er.Message())
					}
				}
			}
		})
	}
}
