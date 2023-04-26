// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"log"

	pb "github.com/opiproject/opi-api/security/v1/gen/go"
)

// Server represents the Server object
type Server struct {
	pb.UnimplementedIPsecServer
}

// IPsecVersion executes the ipsecVersion
func (s *Server) IPsecVersion(_ context.Context, in *pb.IPsecVersionReq) (*pb.IPsecVersionResp, error) {
	log.Printf("IPsecVersion: got %v", in)
	// TODO: replace ipsec -> evpn
	return &pb.IPsecVersionResp{}, nil
}
