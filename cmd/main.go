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

	pe "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/evpn"
	"github.com/opiproject/opi-evpn-bridge/pkg/repository"

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
	// Provision for persistent storage here
	dbvrf, err := repository.VrfFactory("memory")
	if err != nil {
		log.Fatalf("failed to create database: %v", err)
	}
	dbvsvi, err := repository.SviFactory("memory")
	if err != nil {
		log.Fatalf("failed to create database: %v", err)
	}
	log.Printf("todo: use db in opi server (%v) and (%v)", dbvrf, dbvsvi)

	s := grpc.NewServer()
	opi := evpn.NewServer()

	pe.RegisterLogicalBridgeServiceServer(s, opi)
	pe.RegisterBridgePortServiceServer(s, opi)
	pe.RegisterVrfServiceServer(s, opi)
	pe.RegisterSviServiceServer(s, opi)

	reflection.Register(s)

	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
