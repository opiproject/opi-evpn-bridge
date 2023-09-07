// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"reflect"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/vishvananda/netlink"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	pc "github.com/opiproject/opi-api/network/opinetcommon/v1alpha1/gen/go"

	"github.com/opiproject/opi-evpn-bridge/pkg/utils/mocks"
)

var (
	testVrfID   = "opi-vrf8"
	testVrfName = resourceIDToFullName("vrfs", testVrfID)
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

func Test_CreateVrf(t *testing.T) {
	tests := map[string]struct {
		id      string
		in      *pb.Vrf
		out     *pb.Vrf
		errCode codes.Code
		errMsg  string
		exist   bool
		on      func(mockNetlink *mocks.Netlink, errMsg string)
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
			out:     &testVrf,
			errCode: codes.OK,
			errMsg:  "",
			exist:   true,
			on:      nil,
		},
		"valid request empty VNI amd empty Loopback": {
			id: testVrfID,
			in: &pb.Vrf{
				Spec: &pb.VrfSpec{
					LoopbackIpPrefix: &pc.IPPrefix{
						Len: 24,
					},
				},
			},
			out: &pb.Vrf{
				Spec: &pb.VrfSpec{
					LoopbackIpPrefix: &pc.IPPrefix{
						Len: 24,
					},
				},
				Status: &pb.VrfStatus{
					LocalAs:      4,
					RoutingTable: 1000,
					Rmac:         []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
				},
			},
			errCode: codes.OK,
			errMsg:  "",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1000}
				mockNetlink.EXPECT().LinkAdd(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(vrf).Return(nil).Once()
			},
		},
		"failed LinkAdd call": {
			id:      testVrfID,
			in:      &testVrf,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkAdd",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				mockNetlink.EXPECT().LinkAdd(vrf).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetUp call": {
			id:      testVrfID,
			in:      &testVrf,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetUp",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				mockNetlink.EXPECT().LinkAdd(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(vrf).Return(errors.New(errMsg)).Once()
			},
		},
		"failed bridge LinkAdd call": {
			id:      testVrfID,
			in:      &testVrf,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkAdd",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkAdd(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkAdd(bridge).Return(errors.New(errMsg)).Once()
			},
		},
		"failed bridge LinkSetMaster call": {
			id:      testVrfID,
			in:      &testVrf,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetMaster",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkAdd(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkAdd(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(bridge, vrf).Return(errors.New(errMsg)).Once()
			},
		},
		"failed bridge LinkSetHardwareAddr call": {
			id:      testVrfID,
			in:      &testVrf,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetHardwareAddr",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkAdd(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkAdd(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(bridge, vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetHardwareAddr(bridge, mock.Anything).Return(errors.New(errMsg)).Once()
			},
		},
		"failed bridge LinkSetUp call": {
			id:      testVrfID,
			in:      &testVrf,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetUp",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkAdd(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkAdd(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(bridge, vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetHardwareAddr(bridge, mock.Anything).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(bridge).Return(errors.New(errMsg)).Once()
			},
		},
		"failed vxlan LinkAdd call": {
			id:      testVrfID,
			in:      &testVrf,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetUp",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkAdd(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkAdd(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(bridge, vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetHardwareAddr(bridge, mock.Anything).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(bridge).Return(nil).Once()
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkAdd(vxlan).Return(errors.New(errMsg)).Once()
			},
		},
		"failed vxlan LinkSetMaster call": {
			id:      testVrfID,
			in:      &testVrf,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetMaster",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkAdd(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkAdd(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(bridge, vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetHardwareAddr(bridge, mock.Anything).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(bridge).Return(nil).Once()
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkAdd(vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(vxlan, bridge).Return(errors.New(errMsg)).Once()
			},
		},
		"failed vxlan LinkSetUp call": {
			id:      testVrfID,
			in:      &testVrf,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetUp",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkAdd(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkAdd(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(bridge, vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetHardwareAddr(bridge, mock.Anything).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(bridge).Return(nil).Once()
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkAdd(vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(vxlan, bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(vxlan).Return(errors.New(errMsg)).Once()
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
			client := pb.NewVrfServiceClient(conn)

			if tt.exist {
				opi.Vrfs[testVrfName] = protoClone(&testVrf)
				opi.Vrfs[testVrfName].Name = testVrfName
			}
			if tt.out != nil {
				tt.out = protoClone(tt.out)
				tt.out.Name = testVrfName
			}
			if tt.on != nil {
				tt.on(mockNetlink, tt.errMsg)
			}

			request := &pb.CreateVrfRequest{Vrf: tt.in, VrfId: tt.id}
			response, err := client.CreateVrf(ctx, request)
			// TODO: hack the random MAC address for now
			if tt.out != nil && response != nil && tt.out.Status != nil && response.Status != nil {
				response.Status.Rmac = tt.out.Status.Rmac
			}
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
		on      func(mockNetlink *mocks.Netlink, errMsg string)
	}{
		"valid request with unknown key": {
			in:      "unknown-id",
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", resourceIDToFullName("vrfs", "unknown-id")),
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
		"failed LinkByName call": {
			in:      testVrfID,
			out:     &emptypb.Empty{},
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", "vni1000"),
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				mockNetlink.EXPECT().LinkByName(vxlanName).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetDown call": {
			in:      testVrfID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetDown",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vxlan).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkDel call": {
			in:      testVrfID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkDel",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(vxlan).Return(errors.New(errMsg)).Once()
			},
		},
		"failed bridge LinkByName call": {
			in:      testVrfID,
			out:     &emptypb.Empty{},
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", "br1000"),
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(vxlan).Return(nil).Once()
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				mockNetlink.EXPECT().LinkByName(bridgeName).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed bridge LinkSetDown call": {
			in:      testVrfID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetDown",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(vxlan).Return(nil).Once()
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkByName(bridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(bridge).Return(errors.New(errMsg)).Once()
			},
		},
		"failed bridge LinkDel call": {
			in:      testVrfID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkDel",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(vxlan).Return(nil).Once()
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkByName(bridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(bridge).Return(errors.New(errMsg)).Once()
			},
		},
		"failed vrf LinkByName call": {
			in:      testVrfID,
			out:     &emptypb.Empty{},
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", testVrfID),
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(vxlan).Return(nil).Once()
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkByName(bridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkByName(testVrfID).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed vrf LinkSetDown call": {
			in:      testVrfID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetDown",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(vxlan).Return(nil).Once()
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkByName(bridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(bridge).Return(nil).Once()
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				mockNetlink.EXPECT().LinkByName(testVrfID).Return(vrf, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vrf).Return(errors.New(errMsg)).Once()
			},
		},
		"failed vrf LinkDel call": {
			in:      testVrfID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkDel",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(vxlan).Return(nil).Once()
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkByName(bridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(bridge).Return(nil).Once()
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				mockNetlink.EXPECT().LinkByName(testVrfID).Return(vrf, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(vrf).Return(errors.New(errMsg)).Once()
			},
		},
		"successful call": {
			in:      testVrfID,
			out:     &emptypb.Empty{},
			errCode: codes.OK,
			errMsg:  "",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				myip := make(net.IP, 4)
				binary.BigEndian.PutUint32(myip, 167772162)
				vxlanName := fmt.Sprintf("vni%d", *testVrf.Spec.Vni)
				vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*testVrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
				mockNetlink.EXPECT().LinkByName(vxlanName).Return(vxlan, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vxlan).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(vxlan).Return(nil).Once()
				bridgeName := fmt.Sprintf("br%d", *testVrf.Spec.Vni)
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
				mockNetlink.EXPECT().LinkByName(bridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(bridge).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(bridge).Return(nil).Once()
				vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: testVrfID}, Table: 1001}
				mockNetlink.EXPECT().LinkByName(testVrfID).Return(vrf, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(vrf).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(vrf).Return(nil).Once()
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
			client := pb.NewVrfServiceClient(conn)

			fname1 := resourceIDToFullName("vrfs", tt.in)
			opi.Vrfs[testVrfName] = protoClone(&testVrf)
			opi.Vrfs[testVrfName].Name = testVrfName
			if tt.on != nil {
				tt.on(mockNetlink, tt.errMsg)
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
				Name: resourceIDToFullName("vrfs", "unknown-id"),
				Spec: spec,
			},
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", resourceIDToFullName("vrfs", "unknown-id")),
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
			client := pb.NewVrfServiceClient(conn)

			if tt.exist {
				opi.Vrfs[testVrfName] = protoClone(&testVrf)
				opi.Vrfs[testVrfName].Name = testVrfName
			}
			if tt.out != nil {
				tt.out = protoClone(tt.out)
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
			client := pb.NewVrfServiceClient(conn)

			opi.Vrfs[testVrfName] = protoClone(&testVrf)
			opi.Vrfs[testVrfName].Name = testVrfName

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
			client := pb.NewVrfServiceClient(conn)

			opi.Vrfs[testVrfName] = protoClone(&testVrf)
			opi.Vrfs[testVrfName].Name = testVrfName
			opi.Pagination["existing-pagination-token"] = 1

			request := &pb.ListVrfsRequest{PageSize: tt.size, PageToken: tt.token}
			response, err := client.ListVrfs(ctx, request)
			if !equalProtoSlices(response.GetVrfs(), tt.out) {
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
