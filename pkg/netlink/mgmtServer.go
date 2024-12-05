// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package netlink handles the netlink related functionality
package netlink

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	pm "github.com/opiproject/opi-evpn-bridge/pkg/netlink/proto/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Server represents the Server object
type Server struct {
	pm.UnimplementedManagementServiceServer
	Pagination map[string]int
	tracer     trace.Tracer
}

// NewServer creates initialized instance of EVPN Mgmt server
func NewServer() *Server {
	return &Server{
		Pagination: make(map[string]int),
		tracer:     otel.Tracer(""),
	}
}

// RunMgmtGrpcServer start the grpc server for all the components
func RunMgmtGrpcServer(grpcPort uint16) {
	if config.GlobalConfig.Tracer {
		tp := utils.InitTracerProvider("opi-evpn-bridge")
		defer func() {
			if err := tp.Shutdown(context.Background()); err != nil {
				log.Panicf("Tracer Provider Shutdown: %v", err)
			}
		}()
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		log.Panicf("failed to listen: %v", err)
	}

	var serverOptions []grpc.ServerOption
	serverOptions = append(serverOptions,
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.UnaryInterceptor(
			logging.UnaryServerInterceptor(utils.InterceptorLogger(log.Default()),
				logging.WithLogOnEvents(
					logging.StartCall,
					logging.FinishCall,
					logging.PayloadReceived,
					logging.PayloadSent,
				),
			)),
	)
	s := grpc.NewServer(serverOptions...)

	ManagementServer := NewServer()
	pm.RegisterManagementServiceServer(s, ManagementServer)

	reflection.Register(s)

	log.Printf("gRPC server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Panicf("failed to serve: %v", err)
	}
}

// DumpNetlinkDB executes the dumping of netlink database
func (s *Server) DumpNetlinkDB(_ context.Context, in *pm.DumpNetlinkDbRequest) (*pm.DumpNetlinkDbResult, error) {
	if in != nil {
		dump, err := DumpDatabases()
		if err != nil {
			return &pm.DumpNetlinkDbResult{}, err
		}
		return &pm.DumpNetlinkDbResult{Details: dump}, nil
	}
	return &pm.DumpNetlinkDbResult{}, nil
}
