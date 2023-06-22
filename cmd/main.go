// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package main is the main package of the application
package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	pb "github.com/opiproject/opi-api/network/cloud/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/evpn"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	port = flag.Int("port", 50151, "The server port")
)

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	s := grpc.NewServer()
	opi := evpn.NewServer()

	// TODO: replace cloud -> evpn
	pb.RegisterCloudInfraServiceServer(s, opi)

	reflection.Register(s)

	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
