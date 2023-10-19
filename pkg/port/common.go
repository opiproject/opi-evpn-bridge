// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package port is the main package of the application
package port

import (
	"context"
	"fmt"
	"log"
	"net"
	"sort"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	pc "github.com/opiproject/opi-api/network/opinetcommon/v1alpha1/gen/go"
)

func sortBridgePorts(ports []*pb.BridgePort) {
	sort.Slice(ports, func(i int, j int) bool {
		return ports[i].Name < ports[j].Name
	})
}

func resourceIDToFullName(resourceID string) string {
	return fmt.Sprintf("//network.opiproject.org/ports/%s", resourceID)
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
)

func dialer(opi *Server) func(context.Context, string) (net.Conn, error) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()

	pb.RegisterBridgePortServiceServer(server, opi)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatal(err)
		}
	}()

	return func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
}
