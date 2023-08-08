// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"crypto/rand"
	"fmt"

	pb "github.com/opiproject/opi-api/network/cloud/v1alpha1/gen/go"
	pe "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

const (
	tenantbridgeName = "br-tenant"
)

// Server represents the Server object
type Server struct {
	pb.UnimplementedCloudInfraServiceServer
	pe.UnimplementedVrfServiceServer
	pe.UnimplementedLogicalBridgeServiceServer
	pe.UnimplementedBridgePortServiceServer
	Bridges map[string]*pe.LogicalBridge
	Ports   map[string]*pe.BridgePort
	Tunnels map[string]*pb.Tunnel
	Vrfs    map[string]*pe.Vrf
}

// NewServer creates initialized instance of EVPN server
func NewServer() *Server {
	return &Server{
		Bridges: make(map[string]*pe.LogicalBridge),
		Ports:   make(map[string]*pe.BridgePort),
		Tunnels: make(map[string]*pb.Tunnel),
		Vrfs:    make(map[string]*pe.Vrf),
	}
}

func resourceIDToFullName(container string, resourceID string) string {
	return fmt.Sprintf("//network.opiproject.org/%s/%s", container, resourceID)
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
