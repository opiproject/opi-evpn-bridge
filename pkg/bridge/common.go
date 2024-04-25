// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package bridge is the main package of the application
package bridge

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

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils/mocks"
)

func sortLogicalBridges(bridges []*pb.LogicalBridge) {
	sort.Slice(bridges, func(i int, j int) bool {
		return bridges[i].Name < bridges[j].Name
	})
}

func (s *Server) createLogicalBridge(lb *pb.LogicalBridge) (*pb.LogicalBridge, error) {
	// check parameters
	if err := s.validateLogicalBridgeSpec(lb); err != nil {
		return nil, err
	}

	// translation of pb to domain object
	domainLB, err := infradb.NewLogicalBridge(lb)
	if err != nil {
		return nil, err
	}
	// Note: The status of the object will be generated in infraDB operation not here
	if err := infradb.CreateLB(domainLB); err != nil {
		return nil, err
	}
	return domainLB.ToPb(), nil
}

func (s *Server) deleteLogicalBridge(name string) error {
	// Note: The status of the object will be generated in infraDB operation not here
	if err := infradb.DeleteLB(name); err != nil {
		return err
	}
	return nil
}

func (s *Server) getLogicalBridge(name string) (*pb.LogicalBridge, error) {
	domainLB, err := infradb.GetLB(name)
	if err != nil {
		return nil, err
	}
	return domainLB.ToPb(), nil
}

func (s *Server) getAllLogicalBridges() ([]*pb.LogicalBridge, error) {
	lbs := []*pb.LogicalBridge{}
	domainLBs, err := infradb.GetAllLBs()
	if err != nil {
		return nil, err
	}

	for _, domainLB := range domainLBs {
		lbs = append(lbs, domainLB.ToPb())
	}
	return lbs, nil
}

func (s *Server) updateLogicalBridge(lb *pb.LogicalBridge) (*pb.LogicalBridge, error) {
	// check parameters
	if err := s.validateLogicalBridgeSpec(lb); err != nil {
		return nil, err
	}

	// translation of pb to domain object
	domainLB, err := infradb.NewLogicalBridge(lb)
	if err != nil {
		return nil, err
	}
	// Note: The status of the object will be generated in infraDB operation not here
	if err := infradb.UpdateLB(domainLB); err != nil {
		return nil, err
	}
	return domainLB.ToPb(), nil
}

func resourceIDToFullName(resourceID string) string {
	return resourcename.Join(
		"//network.opiproject.org/",
		"bridges", resourceID,
	)
}

// TODO: Move all these functions to a common place and replace them by one function
// for all the objects.
func checkTobeDeletedStatus(lb *pb.LogicalBridge) error {
	if lb.Status.OperStatus == pb.LBOperStatus_LB_OPER_STATUS_TO_BE_DELETED {
		return fmt.Errorf("logical Bridge %s in to be deleted status", lb.Name)
	}

	return nil
}

// TODO: move all of this to a common place

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

// TestCreateLogicalBridge is used for testing purposes
func (s *Server) TestCreateLogicalBridge(lb *pb.LogicalBridge) (*pb.LogicalBridge, error) {
	// check parameters
	if err := s.validateLogicalBridgeSpec(lb); err != nil {
		return nil, err
	}

	// translation of pb to domain object
	domainLB, err := infradb.NewLogicalBridge(lb)
	if err != nil {
		return nil, err
	}
	// Note: The status of the object will be generated in infraDB operation not here
	if err := infradb.CreateLB(domainLB); err != nil {
		return nil, err
	}
	return domainLB.ToPb(), nil
}

func newTestEnv(ctx context.Context, t *testing.T) *testEnv {
	env := &testEnv{}
	env.mockNetlink = mocks.NewNetlink(t)
	env.mockFrr = mocks.NewFrr(t)
	env.opi = NewServer()
	eb := eventbus.EBus
	eb.StartSubscriber("dummy", "logical-bridge", 1, nil)
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

	pb.RegisterLogicalBridgeServiceServer(server, opi)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatal(err)
		}
	}()

	return func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
}
