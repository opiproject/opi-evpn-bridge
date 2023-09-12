// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"log"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	"github.com/philippgille/gokv"
	"github.com/philippgille/gokv/gomap"

	pe "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

func dialer(opi *Server) func(context.Context, string) (net.Conn, error) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()

	pe.RegisterLogicalBridgeServiceServer(server, opi)
	pe.RegisterBridgePortServiceServer(server, opi)
	pe.RegisterVrfServiceServer(server, opi)
	pe.RegisterSviServiceServer(server, opi)

	go func() {
		if err := server.Serve(listener); err != nil {
			log.Fatal(err)
		}
	}()

	return func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
}

func equalProtoSlices[T proto.Message](x, y []T) bool {
	if len(x) != len(y) {
		return false
	}

	for i := 0; i < len(x); i++ {
		if !proto.Equal(x[i], y[i]) {
			return false
		}
	}

	return true
}

func TestFrontEnd_NewServerWithArgs(t *testing.T) {
	tests := map[string]struct {
		frr       utils.Frr
		nLink     utils.Netlink
		store     gokv.Store
		wantPanic bool
	}{
		"nil netlink argument": {
			frr:       &utils.FrrWrapper{},
			nLink:     nil,
			store:     gomap.NewStore(gomap.DefaultOptions),
			wantPanic: true,
		},
		"nil store argument": {
			frr:       &utils.FrrWrapper{},
			nLink:     &utils.NetlinkWrapper{},
			store:     nil,
			wantPanic: true,
		},
		"nil frr argument": {
			frr:       nil,
			nLink:     &utils.NetlinkWrapper{},
			store:     gomap.NewStore(gomap.DefaultOptions),
			wantPanic: true,
		},
		"all valid arguments": {
			frr:       &utils.FrrWrapper{},
			nLink:     &utils.NetlinkWrapper{},
			store:     gomap.NewStore(gomap.DefaultOptions),
			wantPanic: false,
		},
	}

	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("NewServerWithArgs() recover = %v, wantPanic = %v", r, tt.wantPanic)
				}
			}()

			server := NewServerWithArgs(tt.nLink, tt.frr, tt.store)
			if server == nil && !tt.wantPanic {
				t.Error("expected non nil server or panic")
			}
		})
	}
}
