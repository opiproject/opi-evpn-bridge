// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package svi is the main package of the application
package svi

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
	"github.com/opiproject/opi-evpn-bridge/pkg/vrf"
)

func sortSvis(svis []*pb.Svi) {
	sort.Slice(svis, func(i int, j int) bool {
		return svis[i].Name < svis[j].Name
	})
}

func (s *Server) createSvi(svi *pb.Svi) (*pb.Svi, error) {
	// check parameters
	if err := s.validateSviSpec(svi); err != nil {
		return nil, err
	}

	// translation of pb to domain object
	domainSvi, err := infradb.NewSvi(svi)
	if err != nil {
		return nil, err
	}
	// Note: The status of the object will be generated in infraDB operation not here
	if err := infradb.CreateSvi(domainSvi); err != nil {
		return nil, err
	}
	return domainSvi.ToPb(), nil
}

func (s *Server) deleteSvi(name string) error {
	// Note: The status of the object will be generated in infraDB operation not here
	if err := infradb.DeleteSvi(name); err != nil {
		return err
	}
	return nil
}

func (s *Server) getSvi(name string) (*pb.Svi, error) {
	domainSvi, err := infradb.GetSvi(name)
	if err != nil {
		return nil, err
	}
	return domainSvi.ToPb(), nil
}

func (s *Server) getAllSvis() ([]*pb.Svi, error) {
	svis := []*pb.Svi{}
	domainSvis, err := infradb.GetAllSvis()
	if err != nil {
		return nil, err
	}

	for _, domainSvi := range domainSvis {
		svis = append(svis, domainSvi.ToPb())
	}
	return svis, nil
}

func (s *Server) updateSvi(svi *pb.Svi) (*pb.Svi, error) {
	// check parameters
	if err := s.validateSviSpec(svi); err != nil {
		return nil, err
	}

	// translation of pb to domain object
	domainSvi, err := infradb.NewSvi(svi)
	if err != nil {
		return nil, err
	}
	// Note: The status of the object will be generated in infraDB operation not here
	if err := infradb.UpdateSvi(domainSvi); err != nil {
		return nil, err
	}
	return domainSvi.ToPb(), nil
}

func resourceIDToFullName(resourceID string) string {
	return resourcename.Join(
		"//network.opiproject.org/",
		"svis", resourceID,
	)
}

func checkTobeDeletedStatus(svi *pb.Svi) error {
	if svi.Status.OperStatus == pb.SVIOperStatus_SVI_OPER_STATUS_TO_BE_DELETED {
		return fmt.Errorf("SVI %s in to be deleted status", svi.Name)
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
)

type testEnv struct {
	mockNetlink *mocks.Netlink
	mockFrr     *mocks.Frr
	opi         *Server
	lbServer    *bridge.Server
	vrfServer   *vrf.Server
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
	env.vrfServer = vrf.NewServer()
	eb := eventbus.EBus
	eb.StartSubscriber("dummy", "logical-bridge", 1, nil)
	eb.StartSubscriber("dummy", "vrf", 1, nil)
	eb.StartSubscriber("dummy", "svi", 1, nil)
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
