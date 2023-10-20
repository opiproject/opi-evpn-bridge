// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package bridge is the main package of the application
package bridge

import (
	"context"
	"fmt"
	"github.com/opiproject/opi-evpn-bridge/pkg/models"
	"log"
	"net"
	"sort"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

func sortLogicalBridges(bridges []*models.Bridge) {
	sort.Slice(bridges, func(i int, j int) bool {
		return bridges[i].Name < bridges[j].Name
	})
}

func resourceIDToFullName(resourceID string) string {
	return fmt.Sprintf("//network.opiproject.org/bridges/%s", resourceID)
}

// TODO: move all of this to a common place
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
