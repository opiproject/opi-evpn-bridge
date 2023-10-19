// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"crypto/rand"
	"fmt"
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/philippgille/gokv"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

const (
	tenantbridgeName = "br-tenant"
)

// Server represents the Server object
type Server struct {
	pb.UnimplementedVrfServiceServer
	pb.UnimplementedSviServiceServer
	pb.UnimplementedLogicalBridgeServiceServer
	pb.UnimplementedBridgePortServiceServer
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

func resourceIDToFullName(container string, resourceID string) string {
	return fmt.Sprintf("//network.opiproject.org/%s/%s", container, resourceID)
}

func protoClone[T proto.Message](protoStruct T) T {
	return proto.Clone(protoStruct).(T)
}

func generateRandMAC() ([]byte, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("unable to retrieve 6 rnd bytes: %s", err)
	}

	// Set locally administered addresses bit and reset multicast bit
	buf[0] = (buf[0] | 0x02) & 0xfe

	return buf, nil
}

func extractPagination(pageSize int32, pageToken string, pagination map[string]int) (size int, offset int, err error) {
	const (
		maxPageSize     = 250
		defaultPageSize = 50
	)
	switch {
	case pageSize < 0:
		return -1, -1, status.Error(codes.InvalidArgument, "negative PageSize is not allowed")
	case pageSize == 0:
		size = defaultPageSize
	case pageSize > maxPageSize:
		size = maxPageSize
	default:
		size = int(pageSize)
	}
	// fetch offset from the database using opaque token
	offset = 0
	if pageToken != "" {
		var ok bool
		offset, ok = pagination[pageToken]
		if !ok {
			return -1, -1, status.Errorf(codes.NotFound, "unable to find pagination token %s", pageToken)
		}
		log.Printf("Found offset %d from pagination token: %s", offset, pageToken)
	}
	return size, offset, nil
}

func limitPagination[T any](result []T, offset int, size int) ([]T, bool) {
	end := offset + size
	hasMoreElements := false
	if end < len(result) {
		hasMoreElements = true
	} else {
		end = len(result)
	}
	return result[offset:end], hasMoreElements
}
