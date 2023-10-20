// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package bridge is the main package of the application
package bridge

import (
	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/objects"
	"github.com/philippgille/gokv"

	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

// Server represents the Server object
type Server struct {
	pb.UnimplementedLogicalBridgeServiceServer
	*objects.Server
}

// NewServer creates initialized instance of EVPN server
func NewServer(store gokv.Store) *Server {
	return &Server{
		Server: objects.NewServer(store),
	}
}

// NewServerWithArgs creates initialized instance of EVPN server
// with externally created Netlink
func NewServerWithArgs(nLink utils.Netlink, frr utils.Frr, store gokv.Store) *Server {
	return &Server{
		Server: objects.NewServerWithArgs(nLink, frr, store),
	}
}
