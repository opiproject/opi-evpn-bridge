// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package main is the main package of the application
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	pc "github.com/opiproject/opi-api/inventory/v1/gen/go"
	pe "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/bridge"
	"github.com/opiproject/opi-evpn-bridge/pkg/port"
	"github.com/opiproject/opi-evpn-bridge/pkg/svi"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"github.com/opiproject/opi-evpn-bridge/pkg/vrf"
	"github.com/opiproject/opi-smbios-bridge/pkg/inventory"

	"github.com/philippgille/gokv"
	"github.com/philippgille/gokv/redis"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
)

func main() {
	var grpcPort int
	flag.IntVar(&grpcPort, "grpc_port", 50151, "The gRPC server port")

	var httpPort int
	flag.IntVar(&httpPort, "http_port", 8082, "The HTTP server port")

	var tlsFiles string
	flag.StringVar(&tlsFiles, "tls", "", "TLS files in server_cert:server_key:ca_cert format.")

	var redisAddress string
	flag.StringVar(&redisAddress, "redis_addr", "127.0.0.1:6379", "Redis address in ip_address:port format")

	var frrAddress string
	flag.StringVar(&frrAddress, "frr_addr", "127.0.0.1", "Frr address in ip_address format, no port")

	flag.Parse()

	// Create KV store for persistence
	options := redis.DefaultOptions
	options.Address = redisAddress
	options.Codec = utils.ProtoCodec{}
	store, err := redis.NewClient(options)
	if err != nil {
		log.Panic(err)
	}
	defer func(store gokv.Store) {
		err := store.Close()
		if err != nil {
			log.Panic(err)
		}
	}(store)

	go runGatewayServer(grpcPort, httpPort)
	runGrpcServer(grpcPort, tlsFiles, frrAddress, store)
}

func runGrpcServer(grpcPort int, tlsFiles, frrAddress string, store gokv.Store) {
	tp := utils.InitTracerProvider("opi-evpn-bridge")
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Panicf("Tracer Provider Shutdown: %v", err)
		}
	}()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		log.Panicf("failed to listen: %v", err)
	}

	var serverOptions []grpc.ServerOption
	if tlsFiles == "" {
		log.Println("TLS files are not specified. Use insecure connection.")
	} else {
		log.Println("Use TLS certificate files:", tlsFiles)
		config, err := utils.ParseTLSFiles(tlsFiles)
		if err != nil {
			log.Panic("Failed to parse string with tls paths:", err)
		}
		log.Println("TLS config:", config)
		var option grpc.ServerOption
		if option, err = utils.SetupTLSCredentials(config); err != nil {
			log.Panic("Failed to setup TLS:", err)
		}
		serverOptions = append(serverOptions, option)
	}
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

	nLink := utils.NewNetlinkWrapper()
	frr := utils.NewFrrWrapperWithArgs(frrAddress)

	bridgeServer := bridge.NewServerWithArgs(nLink, frr, store)
	portServer := port.NewServerWithArgs(nLink, frr, store)
	vrfServer := vrf.NewServerWithArgs(nLink, frr, store)
	sviServer := svi.NewServerWithArgs(nLink, frr, store)

	pe.RegisterLogicalBridgeServiceServer(s, bridgeServer)
	pe.RegisterBridgePortServiceServer(s, portServer)
	pe.RegisterVrfServiceServer(s, vrfServer)
	pe.RegisterSviServiceServer(s, sviServer)
	pc.RegisterInventoryServiceServer(s, &inventory.Server{})

	reflection.Register(s)

	log.Printf("gRPC server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Panicf("failed to serve: %v", err)
	}
}

func runGatewayServer(grpcPort int, httpPort int) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Register gRPC server endpoint
	// Note: Make sure the gRPC server is running properly and accessible
	mux := runtime.NewServeMux()
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}

	// TODO: add/replace with more/less registrations, once opi-api compiler fixed
	err := pc.RegisterInventoryServiceHandlerFromEndpoint(ctx, mux, fmt.Sprintf(":%d", grpcPort), opts)
	if err != nil {
		log.Panic("cannot register handler server")
	}

	// Start HTTP server (and proxy calls to gRPC server endpoint)
	log.Printf("HTTP Server listening at %v", httpPort)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", httpPort),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	err = server.ListenAndServe()
	if err != nil {
		log.Panic("cannot start HTTP gateway server")
	}
}
