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
	"sort"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

func sortVrfs(vrfs []*pb.Vrf) {
	sort.Slice(vrfs, func(i int, j int) bool {
		return vrfs[i].Name < vrfs[j].Name
	})
}

func resourceIDToFullName(resourceID string) string {
	return fmt.Sprintf("//network.opiproject.org/vrfs/%s", resourceID)
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

// TODO: move all of this to a common place
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
