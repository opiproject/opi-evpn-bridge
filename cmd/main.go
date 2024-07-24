// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package main is the main package of the application
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	pc "github.com/opiproject/opi-api/inventory/v1/gen/go"
	pe "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
	"github.com/opiproject/opi-evpn-bridge/pkg/bridge"
	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/taskmanager"
	"github.com/opiproject/opi-evpn-bridge/pkg/port"
	"github.com/opiproject/opi-evpn-bridge/pkg/svi"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"github.com/opiproject/opi-evpn-bridge/pkg/vrf"
	"github.com/opiproject/opi-smbios-bridge/pkg/inventory"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	ci_linux "github.com/opiproject/opi-evpn-bridge/pkg/LinuxCIModule"
	gen_linux "github.com/opiproject/opi-evpn-bridge/pkg/LinuxGeneralModule"
	intel_e2000_linux "github.com/opiproject/opi-evpn-bridge/pkg/LinuxVendorModule/intele2000"
	frr "github.com/opiproject/opi-evpn-bridge/pkg/frr"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
)

var rootCmd = &cobra.Command{
	Use:   "opi-evpn-bridge",
	Short: "evpn bridge",
	Long:  "evpn bridge application",

	Run: func(_ *cobra.Command, _ []string) {

		taskmanager.TaskMan.StartTaskManager()

		err := infradb.NewInfraDB(config.GlobalConfig.DBAddress, config.GlobalConfig.Database)
		if err != nil {
			log.Panicf("Error: %v", err)
		}
		go runGatewayServer(config.GlobalConfig.GRPCPort, config.GlobalConfig.HTTPPort)

		switch config.GlobalConfig.Buildenv {
		case "intel_e2000":
			gen_linux.Init()
			intel_e2000_linux.Init()
			frr.Init()
		case "ci":
			gen_linux.Init()
			ci_linux.Init()
			frr.Init()
		default:
			log.Panic(" ERROR: Could not find Build env ")
		}

		// Create GRD VRF configuration during startup
		if err := createGrdVrf(); err != nil {
			log.Panicf("Error: %v", err)
		}

		runGrpcServer(config.GlobalConfig.GRPCPort, config.GlobalConfig.TLSFiles)
	},
}

// initialize the cobra configuration and bind the flags
func initialize() error {
	cobra.OnInitialize(config.Initcfg)

	rootCmd.PersistentFlags().StringVarP(&config.GlobalConfig.CfgFile, "config", "c", "config.yaml", "config file path")
	rootCmd.PersistentFlags().Uint16Var(&config.GlobalConfig.GRPCPort, "grpcport", 50151, "The gRPC server port")
	rootCmd.PersistentFlags().Uint16Var(&config.GlobalConfig.HTTPPort, "httpport", 8082, "The HTTP server port")
	rootCmd.PersistentFlags().StringVar(&config.GlobalConfig.TLSFiles, "tlsfiles", "", "TLS files in server_cert:server_key:ca_cert format.")
	rootCmd.PersistentFlags().StringVar(&config.GlobalConfig.DBAddress, "dbaddress", "127.0.0.1:6379", "db address in ip_address:port format")
	rootCmd.PersistentFlags().StringVar(&config.GlobalConfig.Database, "database", "redis", "Database connection string")

	// Bind command-line flags to config fields
	if err := viper.GetViper().BindPFlags(rootCmd.PersistentFlags()); err != nil {
		log.Printf("Error binding flags to Viper: %v\n", err)
		return err
	}

	return nil
}

const logfile string = "opi-evpn-bridge.log"

var logger *log.Logger

// setupLogger sets the config for logger
func setupLogger(filename string) {
	var err error
	filename = filepath.Clean(filename)
	out, err := os.OpenFile(filename, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		log.Panic(err)
	}
	logger = log.New(io.MultiWriter(out), "", log.Lshortfile|log.LstdFlags)
	log.SetOutput(logger.Writer())
}

// main function
func main() {
	// setup file and console logger
	setupLogger(logfile)

	// initialize  cobra config
	if err := initialize(); err != nil {
		// log.Println(err)
		log.Panicf("Error in initialize(): %v", err)
	}

	// start the main cmd
	if err := rootCmd.Execute(); err != nil {
		log.Panicf("Error in Execute(): %v", err)
	}

	defer func() {
		if err := infradb.Close(); err != nil {
			log.Panicf("Error in close(): %v", err)
		}
	}()
}

// runGrpcServer start the grpc server for all the components
func runGrpcServer(grpcPort uint16, tlsFiles string) {
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

	bridgeServer := bridge.NewServer()
	portServer := port.NewServer()
	vrfServer := vrf.NewServer()
	sviServer := svi.NewServer()
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

// runGatewayServer
func runGatewayServer(grpcPort uint16, httpPort uint16) {
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

// createGrdVrf creates the grd vrf with vni 0
func createGrdVrf() error {
	grdVrf, err := infradb.NewVrfWithArgs("//network.opiproject.org/vrfs/GRD", nil, nil, nil)
	if err != nil {
		log.Printf("CreateGrdVrf(): Error in initializing GRD VRF object %+v\n", err)
		return err
	}

	err = infradb.CreateVrf(grdVrf)
	if err != nil {
		log.Printf("CreateGrdVrf(): Error in creating GRD VRF object %+v\n", err)
		return err
	}

	return nil
}
