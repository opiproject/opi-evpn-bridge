// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
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

	"github.com/opiproject/opi-evpn-bridge/pkg/utils/mocks"
)

var (
	testBridgePortID   = "opi-port8"
	testBridgePortName = resourceIDToFullName("ports", testBridgePortID)
	testBridgePort     = pb.BridgePort{
		Spec: &pb.BridgePortSpec{
			MacAddress:     []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
			Ptype:          pb.BridgePortType_TRUNK,
			LogicalBridges: []string{testLogicalBridgeName},
		},
	}
	testBridgePortWithStatus = pb.BridgePort{
		Name: testBridgePortName,
		Spec: testBridgePort.Spec,
		Status: &pb.BridgePortStatus{
			OperStatus: pb.BPOperStatus_BP_OPER_STATUS_UP,
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
		on      func(mockNetlink *mocks.Netlink, errMsg string)
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
			out:     &testBridgePort,
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
					Ptype:          pb.BridgePortType_ACCESS,
					LogicalBridges: []string{"Japan", "Australia", "Germany"},
				},
			},
			out:     nil,
			errCode: codes.InvalidArgument,
			errMsg:  fmt.Sprintf("ACCESS type must have single LogicalBridge and not (%d)", 3),
			exist:   false,
			on:      nil,
		},
		"failed LinkByName bridge call": {
			id:      testBridgePortID,
			in:      &testBridgePort,
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", tenantbridgeName),
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed LinkByName port call": {
			id:      testBridgePortID,
			in:      &testBridgePort,
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", testBridgePortID),
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(bridge, nil).Once()
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetHardwareAddr call": {
			id:      testBridgePortID,
			in:      &testBridgePort,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetHardwareAddr",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(bridge, nil).Once()
				iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: testBridgePortID}}
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(iface, nil).Once()
				mac := net.HardwareAddr(testBridgePort.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(iface, mac).Return(errors.New(errMsg)).Once()
			},
		},
		"failed bridge LinkSetMaster call": {
			id:      testBridgePortID,
			in:      &testBridgePort,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetMaster",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(bridge, nil).Once()
				iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: testBridgePortID}}
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(iface, nil).Once()
				mac := net.HardwareAddr(testBridgePort.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(iface, mac).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(mock.Anything, mock.Anything).Return(errors.New(errMsg)).Once()
			},
		},
		"missing bridges": {
			id: testBridgePortID,
			in: &pb.BridgePort{
				Spec: &pb.BridgePortSpec{
					MacAddress:     []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
					Ptype:          pb.BridgePortType_TRUNK,
					LogicalBridges: []string{"Japan", "Australia", "Germany"},
				},
			},
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", "Japan"),
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(bridge, nil).Once()
				iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: testBridgePortID}}
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(iface, nil).Once()
				mac := net.HardwareAddr(testBridgePort.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(iface, mac).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(iface, bridge).Return(nil).Once()
			},
		},
		"failed BridgeVlanAdd TRUNK call": {
			id:      testBridgePortID,
			in:      &testBridgePort,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call BridgeVlanAdd",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(bridge, nil).Once()
				iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: testBridgePortID}}
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(iface, nil).Once()
				mac := net.HardwareAddr(testBridgePort.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(iface, mac).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(iface, bridge).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanAdd(iface, vid, false, false, false, false).Return(errors.New(errMsg)).Once()
			},
		},
		"failed BridgeVlanAdd ACCESS call": {
			id: testBridgePortID,
			in: &pb.BridgePort{
				Spec: &pb.BridgePortSpec{
					MacAddress:     []byte{0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F},
					Ptype:          pb.BridgePortType_ACCESS,
					LogicalBridges: []string{testLogicalBridgeName},
				},
			},
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call BridgeVlanAdd",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(bridge, nil).Once()
				iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: testBridgePortID}}
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(iface, nil).Once()
				mac := net.HardwareAddr(testBridgePort.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(iface, mac).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(iface, bridge).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanAdd(iface, vid, true, true, false, false).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetUp call": {
			id:      testBridgePortID,
			in:      &testBridgePort,
			out:     nil,
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetUp",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(bridge, nil).Once()
				iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: testBridgePortID}}
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(iface, nil).Once()
				mac := net.HardwareAddr(testBridgePort.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(iface, mac).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(iface, bridge).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanAdd(iface, vid, false, false, false, false).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(iface).Return(errors.New(errMsg)).Once()
			},
		},
		"successful call": {
			id:      testBridgePortID,
			in:      &testBridgePort,
			out:     &testBridgePortWithStatus,
			errCode: codes.OK,
			errMsg:  "",
			exist:   false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: tenantbridgeName}}
				mockNetlink.EXPECT().LinkByName(tenantbridgeName).Return(bridge, nil).Once()
				iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: testBridgePortID}}
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(iface, nil).Once()
				mac := net.HardwareAddr(testBridgePort.Spec.MacAddress[:])
				mockNetlink.EXPECT().LinkSetHardwareAddr(iface, mac).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetMaster(iface, bridge).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanAdd(iface, vid, false, false, false, false).Return(nil).Once()
				mockNetlink.EXPECT().LinkSetUp(iface).Return(nil).Once()
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
			client := pb.NewBridgePortServiceClient(conn)

			opi.Bridges[testLogicalBridgeName] = protoClone(&testLogicalBridge)
			opi.Bridges[testLogicalBridgeName].Name = testLogicalBridgeName
			if tt.exist {
				opi.Ports[testBridgePortName] = protoClone(&testBridgePort)
				opi.Ports[testBridgePortName].Name = testBridgePortName
			}
			if tt.out != nil {
				tt.out = protoClone(tt.out)
				tt.out.Name = testBridgePortName
			}
			if tt.on != nil {
				tt.on(mockNetlink, tt.errMsg)
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
		on      func(mockNetlink *mocks.Netlink, errMsg string)
	}{
		"valid request with unknown key": {
			in:      "unknown-id",
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", resourceIDToFullName("ports", "unknown-id")),
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
			in:      testBridgePortID,
			out:     &emptypb.Empty{},
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", testBridgePortID),
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(nil, errors.New(errMsg)).Once()
			},
		},
		"failed LinkSetDown call": {
			in:      testBridgePortID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkSetDown",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: testBridgePortID}}
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(iface, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(iface).Return(errors.New(errMsg)).Once()
			},
		},
		"failed BridgeVlanDel call": {
			in:      testBridgePortID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call BridgeVlanDel",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: testBridgePortID}}
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(iface, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(iface).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanDel(iface, vid, true, true, false, false).Return(errors.New(errMsg)).Once()
			},
		},
		"failed LinkDel call": {
			in:      testBridgePortID,
			out:     &emptypb.Empty{},
			errCode: codes.Unknown,
			errMsg:  "Failed to call LinkDel",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: testBridgePortID}}
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(iface, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(iface).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanDel(iface, vid, true, true, false, false).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(iface).Return(errors.New(errMsg)).Once()
			},
		},
		"successful call": {
			in:      testBridgePortID,
			out:     &emptypb.Empty{},
			errCode: codes.OK,
			errMsg:  "",
			missing: false,
			on: func(mockNetlink *mocks.Netlink, errMsg string) {
				iface := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: testBridgePortID}}
				mockNetlink.EXPECT().LinkByName(testBridgePortID).Return(iface, nil).Once()
				mockNetlink.EXPECT().LinkSetDown(iface).Return(nil).Once()
				vid := uint16(testLogicalBridge.Spec.VlanId)
				mockNetlink.EXPECT().BridgeVlanDel(iface, vid, true, true, false, false).Return(nil).Once()
				mockNetlink.EXPECT().LinkDel(iface).Return(nil).Once()
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
			client := pb.NewBridgePortServiceClient(conn)

			fname1 := resourceIDToFullName("ports", tt.in)
			opi.Ports[testBridgePortName] = protoClone(&testBridgePort)
			opi.Ports[testBridgePortName].Name = testBridgePortName
			opi.Bridges[testLogicalBridgeName] = protoClone(&testLogicalBridge)
			opi.Bridges[testLogicalBridgeName].Name = testLogicalBridgeName
			if tt.on != nil {
				tt.on(mockNetlink, tt.errMsg)
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
		Ptype:          pb.BridgePortType_ACCESS,
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
				Name: resourceIDToFullName("ports", "unknown-id"),
				Spec: spec,
			},
			out:     nil,
			errCode: codes.NotFound,
			errMsg:  fmt.Sprintf("unable to find key %v", resourceIDToFullName("ports", "unknown-id")),
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
			client := pb.NewBridgePortServiceClient(conn)

			if tt.exist {
				opi.Ports[testBridgePortName] = protoClone(&testBridgePort)
				opi.Ports[testBridgePortName].Name = testBridgePortName
			}
			if tt.out != nil {
				tt.out = protoClone(tt.out)
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
			client := pb.NewBridgePortServiceClient(conn)

			opi.Ports[testBridgePortName] = protoClone(&testBridgePort)
			opi.Ports[testBridgePortName].Name = testBridgePortName

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
			client := pb.NewBridgePortServiceClient(conn)

			opi.Ports[testBridgePortName] = protoClone(&testBridgePort)
			opi.Ports[testBridgePortName].Name = testBridgePortName
			opi.Pagination["existing-pagination-token"] = 1

			request := &pb.ListBridgePortsRequest{PageSize: tt.size, PageToken: tt.token}
			response, err := client.ListBridgePorts(ctx, request)
			if !equalProtoSlices(response.GetBridgePorts(), tt.out) {
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
