// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"fmt"

	pb "github.com/opiproject/opi-api/network/cloud/v1alpha1/gen/go"
)

// Server represents the Server object
type Server struct {
	pb.UnimplementedCloudInfraServiceServer
	Subnets    map[string]*pb.Subnet
	Interfaces map[string]*pb.Interface
	Tunnels    map[string]*pb.Tunnel
	Vpcs       map[string]*pb.Vpc
}

// NewServer creates initialized instance of EVPN server
func NewServer() *Server {
	return &Server{
		Subnets:    make(map[string]*pb.Subnet),
		Interfaces: make(map[string]*pb.Interface),
		Tunnels:    make(map[string]*pb.Tunnel),
		Vpcs:       make(map[string]*pb.Vpc),
	}
}

func resourceIDToFullName(container string, resourceID string) string {
	return fmt.Sprintf("//network.opiproject.org/%s/%s", container, resourceID)
}
