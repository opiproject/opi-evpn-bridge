// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2023 Intel Corporation

// Package utils contains utility functions
package utils

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// TLSConfig contains information required to enable TLS for gRPC server.
type TLSConfig struct {
	ServerCertPath string
	ServerKeyPath  string
	CaCertPath     string
}

// ParseTLSFiles parses a string containing server certificate,
// server key and CA certificate separated by `:`
func ParseTLSFiles(tlsFiles string) (TLSConfig, error) {
	files := strings.Split(tlsFiles, ":")

	numOfFiles := len(files)
	if numOfFiles != 3 {
		return TLSConfig{}, errors.New("wrong number of path entries provided." +
			"Expect <server cert>:<server key>:<ca cert> are provided separated by `:`")
	}

	tls := TLSConfig{}

	const emptyPathErr = "empty %s path is not allowed"
	tls.ServerCertPath = files[0]
	if tls.ServerCertPath == "" {
		return TLSConfig{}, fmt.Errorf(emptyPathErr, "server cert")
	}

	tls.ServerKeyPath = files[1]
	if tls.ServerKeyPath == "" {
		return TLSConfig{}, fmt.Errorf(emptyPathErr, "server key")
	}

	tls.CaCertPath = files[2]
	if tls.CaCertPath == "" {
		return TLSConfig{}, fmt.Errorf(emptyPathErr, "CA cert")
	}

	return tls, nil
}

// SetupTLSCredentials returns a service options to enable TLS for gRPC server
func SetupTLSCredentials(config TLSConfig) (grpc.ServerOption, error) {
	return setupTLSCredentials(config, tls.LoadX509KeyPair, os.ReadFile)
}

func setupTLSCredentials(config TLSConfig,
	loadX509KeyPair func(string, string) (tls.Certificate, error),
	readFile func(string) ([]byte, error),
) (grpc.ServerOption, error) {
	serverCert, err := loadX509KeyPair(config.ServerCertPath, config.ServerKeyPath)
	if err != nil {
		return nil, err
	}

	c := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
	}

	c.ClientCAs = x509.NewCertPool()
	log.Println("Loading client ca certificate:", config.CaCertPath)

	clientCaCert, err := readFile(config.CaCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate: %v. error: %v", config.CaCertPath, err)
	}

	if !c.ClientCAs.AppendCertsFromPEM(clientCaCert) {
		return nil, fmt.Errorf("failed to add client CA's certificate: %v", config.CaCertPath)
	}

	return grpc.Creds(credentials.NewTLS(c)), nil
}
