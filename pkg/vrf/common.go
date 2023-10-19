// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package vrf is the main package of the application
package vrf

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

// TODO: move all of this to a common place
func resourceIDToFullName(_ string, resourceID string) string {
	return fmt.Sprintf("//network.opiproject.org/vrfs/%s", resourceID)
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

func dialer(opi *Server) func(context.Context, string) (net.Conn, error) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()

	pb.RegisterVrfServiceServer(server, opi)

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
