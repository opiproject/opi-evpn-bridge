// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package port is the main package of the application
package port

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

// Server represents the Server object
type Server struct {
	pb.UnimplementedBridgePortServiceServer
	Pagination map[string]int
	tracer     trace.Tracer
}

// NewServer creates initialized instance of EVPN server
func NewServer() *Server {
	return &Server{
		Pagination: make(map[string]int),
		tracer:     otel.Tracer(""),
	}
}
