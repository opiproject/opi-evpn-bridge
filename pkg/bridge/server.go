// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package bridge is the main package of the application
package bridge

import (
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/philippgille/gokv"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

// Server represents the Server object
type Server struct {
	pb.UnimplementedLogicalBridgeServiceServer
	Pagination map[string]int
	ListHelper map[string]bool
	nLink      utils.Netlink
	frr        utils.Frr
	tracer     trace.Tracer
	store      gokv.Store
}

// NewServer creates initialized instance of EVPN server
func NewServer(store gokv.Store) *Server {
	nLink := utils.NewNetlinkWrapper()
	frr := utils.NewFrrWrapper()
	return NewServerWithArgs(nLink, frr, store)
}

// NewServerWithArgs creates initialized instance of EVPN server
// with externally created Netlink
func NewServerWithArgs(nLink utils.Netlink, frr utils.Frr, store gokv.Store) *Server {
	if frr == nil {
		log.Panic("nil for Frr is not allowed")
	}
	if nLink == nil {
		log.Panic("nil for Netlink is not allowed")
	}
	if store == nil {
		log.Panic("nil for Store is not allowed")
	}
	return &Server{
		ListHelper: make(map[string]bool),
		Pagination: make(map[string]int),
		nLink:      nLink,
		frr:        frr,
		tracer:     otel.Tracer(""),
		store:      store,
	}
}
