// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package port is the main package of the application
package port

import (
	"context"
	"fmt"
	"log"
	"net"
	"sort"
	"testing"

	"go.einride.tech/aip/resourcename"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	pc "github.com/opiproject/opi-api/network/opinetcommon/v1alpha1/gen/go"

	"github.com/opiproject/opi-evpn-bridge/pkg/bridge"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils/mocks"
)

func sortBridgePorts(ports []*pb.BridgePort) {
	sort.Slice(ports, func(i int, j int) bool {
		return ports[i].Name < ports[j].Name
	})
}

func (s *Server) createBridgePort(bp *pb.BridgePort) (*pb.BridgePort, error) {
	// check parameters
	if err := s.validateBridgePortSpec(bp); err != nil {
		return nil, err
	}

	// translation of pb to domain object
	domainBP := infradb.NewBridgePort(bp)
	// Note: The status of the object will be generated in infraDB operation not here
	if err := infradb.CreateBP(domainBP); err != nil {
		return nil, err
	}
	return domainBP.ToPb(), nil
}

func (s *Server) deleteBridgePort(name string) error {
	// Note: The status of the object will be generated in infraDB operation not here
	if err := infradb.DeleteBP(name); err != nil {
		return err
	}
	return nil
}

func (s *Server) getBridgePort(name string) (*pb.BridgePort, error) {
	domainBP, err := infradb.GetBP(name)
	if err != nil {
		return nil, err
	}
	return domainBP.ToPb(), nil
}

func (s *Server) getAllBridgePorts() ([]*pb.BridgePort, error) {
	bps := []*pb.BridgePort{}
	domainBPs, err := infradb.GetAllBPs()
	if err != nil {
		return nil, err
	}

	for _, domainBP := range domainBPs {
		bps = append(bps, domainBP.ToPb())
	}
	return bps, nil
}

func (s *Server) updateBridgePort(bp *pb.BridgePort) (*pb.BridgePort, error) {
	// check parameters
	if err := s.validateBridgePortSpec(bp); err != nil {
		return nil, err
	}

	// translation of pb to domain object
	domainBP := infradb.NewBridgePort(bp)
	// Note: The status of the object will be generated in infraDB operation not here
	if err := infradb.UpdateBP(domainBP); err != nil {
		return nil, err
	}
	return domainBP.ToPb(), nil
}

func resourceIDToFullName(resourceID string) string {
	return resourcename.Join(
		"//network.opiproject.org/",
		"ports", resourceID,
	)
}

func checkTobeDeletedStatus(bp *pb.BridgePort) error {
	if bp.Status.OperStatus == pb.BPOperStatus_BP_OPER_STATUS_TO_BE_DELETED {
		return fmt.Errorf("bridge Port %s in to be deleted status", bp.Name)
	}

	return nil
}

// TODO: move all of this to a common place

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
)

type testEnv struct {
	mockNetlink *mocks.Netlink
	mockFrr     *mocks.Frr
	opi         *Server
	lbServer    *bridge.Server
	conn        *grpc.ClientConn
}

func (e *testEnv) Close() {
	err := e.conn.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func newTestEnv(ctx context.Context, t *testing.T) *testEnv {
	env := &testEnv{}
	env.mockNetlink = mocks.NewNetlink(t)
	env.mockFrr = mocks.NewFrr(t)
	env.opi = NewServer()
	env.lbServer = bridge.NewServer()
	eb := eventbus.EBus
	eb.StartSubscriber("dummy", "logical-bridge", 1, nil)
	eb.StartSubscriber("dummy", "bridge-port", 1, nil)
	_ = infradb.NewInfraDB("", "gomap")
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
