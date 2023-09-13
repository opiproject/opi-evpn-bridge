// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2023 Intel Corporation

// Package utils contains utility functions
package utils

import (
	"crypto/tls"
	"errors"
	"testing"
)

func TestServer_ParseTLSFiles(t *testing.T) {
	tests := map[string]struct {
		tlsStr     string
		expectErr  bool
		serverCert string
		serverKey  string
		caCert     string
	}{
		"no files are provided": {
			tlsStr:     "",
			expectErr:  true,
			serverCert: "",
			serverKey:  "",
			caCert:     "",
		},
		"1 file is provided": {
			tlsStr:     "a",
			expectErr:  true,
			serverCert: "",
			serverKey:  "",
			caCert:     "",
		},
		"2 files are provided": {
			tlsStr:     "a:b",
			expectErr:  true,
			serverCert: "",
			serverKey:  "",
			caCert:     "",
		},
		"3 files are provided": {
			tlsStr:     "a:b:c",
			expectErr:  false,
			serverCert: "a",
			serverKey:  "b",
			caCert:     "c",
		},
		"more files are provided then expected": {
			tlsStr:     "a:b:c:d",
			expectErr:  true,
			serverCert: "",
			serverKey:  "",
			caCert:     "",
		},
		"empty CA cert file path": {
			tlsStr:     "a:b:",
			expectErr:  true,
			serverCert: "",
			serverKey:  "",
			caCert:     "",
		},
		"empty server key file path": {
			tlsStr:     "a::c",
			expectErr:  true,
			serverCert: "",
			serverKey:  "",
			caCert:     "",
		},
		"empty server cert file path": {
			tlsStr:     ":b:c",
			expectErr:  true,
			serverCert: "",
			serverKey:  "",
			caCert:     "",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			config, err := ParseTLSFiles(tt.tlsStr)

			if (err != nil) != tt.expectErr {
				t.Error("Expect error", tt.expectErr, "received", err)
			}
			if !tt.expectErr {
				if config.ServerCertPath != tt.serverCert {
					t.Error("Expect", tt.serverCert, "received", config.ServerCertPath)
				}
				if config.ServerKeyPath != tt.serverKey {
					t.Error("Expect", tt.serverKey, "received", config.ServerKeyPath)
				}
				if config.CaCertPath != tt.caCert {
					t.Error("Expect", tt.caCert, "received", config.CaCertPath)
				}
			}
		})
	}
}

var validCa = []byte(
	`-----BEGIN CERTIFICATE-----
MIICYzCCAg2gAwIBAgIUXcH7z871xc1nvqiV80yiCsmrZIkwDQYJKoZIhvcNAQEL
BQAwgYQxCzAJBgNVBAYTAlBMMRQwEgYDVQQIDAtNYXpvd2llY2tpZTERMA8GA1UE
BwwIV2Fyc3phd2ExDjAMBgNVBAoMBUludGVsMQwwCgYDVQQLDANPUEkxEjAQBgNV
BAMMCSoub3BpLmNvbTEaMBgGCSqGSIb3DQEJARYLb3BpQG9waS5jb20wIBcNMjMw
NTIyMTQyMDQyWhgPMjEyMzA0MjgxNDIwNDJaMIGEMQswCQYDVQQGEwJQTDEUMBIG
A1UECAwLTWF6b3dpZWNraWUxETAPBgNVBAcMCFdhcnN6YXdhMQ4wDAYDVQQKDAVJ
bnRlbDEMMAoGA1UECwwDT1BJMRIwEAYDVQQDDAkqLm9waS5jb20xGjAYBgkqhkiG
9w0BCQEWC29waUBvcGkuY29tMFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBALhaOwyJ
DfVdUi7zlQiZNMPEEVHkdR6ougIND3c+UbWnR3oFyn7a7YOb+jnjWp18DZcqlVES
q5H0SBjyzd9dR2MCAwEAAaNTMFEwHQYDVR0OBBYEFN/LaFbmoEvAnCgJ4+xfQwK5
mdF2MB8GA1UdIwQYMBaAFN/LaFbmoEvAnCgJ4+xfQwK5mdF2MA8GA1UdEwEB/wQF
MAMBAf8wDQYJKoZIhvcNAQELBQADQQCnrnBr01nNKJYHodOqHq5mJWSCDNb9Fv8S
L8TMNwAmhE2vO1JRpFUgPIwaTnAPFLriYQpLW7JbHQos0wRS+vJZ
-----END CERTIFICATE-----
`)

func TestServer_SetupTLSCredentials(t *testing.T) {
	tests := map[string]struct {
		config      TLSConfig
		expectErr   bool
		loadKeyErr  error
		readFileErr error
		validCaCert bool
	}{
		"failed to load key pair": {
			expectErr:   true,
			loadKeyErr:  errors.New("Key load failed"),
			readFileErr: nil,
			validCaCert: true,
		},
		"failed to read file": {
			expectErr:   true,
			loadKeyErr:  nil,
			readFileErr: errors.New("Failed to read file"),
			validCaCert: true,
		},
		"invalid CA certificate": {
			expectErr:   true,
			loadKeyErr:  nil,
			readFileErr: nil,
			validCaCert: false,
		},
		"valid CA certificate": {
			expectErr:   false,
			loadKeyErr:  nil,
			readFileErr: nil,
			validCaCert: true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			caCert := make([]byte, len(validCa))
			copy(caCert, validCa)
			if !tt.validCaCert {
				caCert[0] = caCert[0] - 1
			}

			out, err := setupTLSCredentials(TLSConfig{
				ServerCertPath: "a",
				ServerKeyPath:  "b",
				CaCertPath:     "c",
			}, func(s1, s2 string) (tls.Certificate, error) {
				return tls.Certificate{}, tt.loadKeyErr
			}, func(s string) ([]byte, error) {
				return caCert, tt.readFileErr
			})

			if (err != nil) != tt.expectErr {
				t.Error("Expect error", tt.expectErr, "received", err)
			}
			if !tt.expectErr && out == nil {
				t.Error("Expect not nil server option, received nil")
			}
		})
	}
}
