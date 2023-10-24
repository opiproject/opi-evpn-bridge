// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package svi is the main package of the application
package svi

import (
	"context"
	"fmt"
	"log"
	"net"
	"sort"
	"testing"

	"github.com/philippgille/gokv/gomap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	pc "github.com/opiproject/opi-api/network/opinetcommon/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils/mocks"
)

func sortSvis(svis []*pb.Svi) {
	sort.Slice(svis, func(i int, j int) bool {
		return svis[i].Name < svis[j].Name
	})
}

func resourceIDToFullName(resourceID string) string {
	return fmt.Sprintf("//network.opiproject.org/svis/%s", resourceID)
}

// TODO: move all of this to a common place
const (
	tenantbridgeName = "br-tenant"
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

	testVrfID   = "opi-vrf8"
	testVrfName = resourceIDToFullName(testVrfID)
	testVrf     = pb.Vrf{
		Spec: &pb.VrfSpec{
			Vni: proto.Uint32(1000),
			LoopbackIpPrefix: &pc.IPPrefix{
				// Addr: &pc.IPAddress{
				// 	Af: pc.IpAf_IP_AF_INET,
				// 	V4OrV6: &pc.IPAddress_V4Addr{
				// 		V4Addr: 167772162,
				// 	},
				// },
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
			LocalAs: 4,
		},
	}
)

type testEnv struct {
	mockNetlink *mocks.Netlink
	mockFrr     *mocks.Frr
	opi         *Server
	conn        *grpc.ClientConn
}

func (e *testEnv) Close() {
	err := e.conn.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func newTestEnv(ctx context.Context, t *testing.T) *testEnv {
	store := gomap.NewStore(gomap.Options{Codec: utils.ProtoCodec{}})
	env := &testEnv{}
	env.mockNetlink = mocks.NewNetlink(t)
	env.mockFrr = mocks.NewFrr(t)
	env.opi = NewServerWithArgs(env.mockNetlink, env.mockFrr, store)
	conn, err := grpc.DialContext(ctx,
		"",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(dialer(env.opi)))
	if err != nil {
		log.Fatal(err)
	}
	env.conn = conn
	return env
}

func dialer(opi *Server) func(context.Context, string) (net.Conn, error) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()

	pb.RegisterSviServiceServer(server, opi)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatal(err)
		}
	}()

	return func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
}
