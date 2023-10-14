// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package utils contails useful helper functions
package utils

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ziutek/telnet"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const (
	network  = "tcp"
	password = "opi"
	address  = "localhost"
	timeout  = 10 * time.Second
)

// Ports defined here https://docs.frrouting.org/en/latest/setup.html#servicess
const (
	zebrasrv = iota + 2600
	zebra
	ripd
	ripngd
	ospfd
	bgpd
	ospf6d
	ospfapi
	isisd
	babeld
	nhrpd
	pimd
	ldpd
	eigprd
	bfdd
	fabricd
	vrrpd
)

// default tracer name is good for now
var tracer = otel.Tracer("")

// FrrZebraCmd connects to Zebra telnet with password and runs command
func FrrZebraCmd(ctx context.Context, command string) (string, error) {
	// ports defined here https://docs.frrouting.org/en/latest/setup.html#services
	return TelnetDialAndCommunicate(ctx, command, zebra)
}

// FrrBgpCmd connects to Bgp telnet with password and runs command
func FrrBgpCmd(ctx context.Context, command string) (string, error) {
	// ports defined here https://docs.frrouting.org/en/latest/setup.html#services
	return TelnetDialAndCommunicate(ctx, command, bgpd)
}

// TelnetDialAndCommunicate connects to telnet with password and runs command
func TelnetDialAndCommunicate(ctx context.Context, command string, port int) (string, error) {
	_, childSpan := tracer.Start(ctx, "frr.Command")
	childSpan.SetAttributes(attribute.String("command.name", command))
	defer childSpan.End()

	// new connection every time
	conn, err := telnet.DialTimeout(network, fmt.Sprintf("%s:%d", address, port), timeout)
	if err != nil {
		return "", err
	}
	defer func(t *telnet.Conn) { _ = t.Close() }(conn)

	conn.SetUnixWriteMode(true)

	err = conn.SetWriteDeadline(time.Now().Add(timeout))
	if err != nil {
		return "", err
	}

	// login
	err = conn.SkipUntil("Password: ")
	if err != nil {
		return "", err
	}
	_, err = conn.Write([]byte(password + "\n"))
	if err != nil {
		return "", err
	}
	err = conn.SkipUntil(">")
	if err != nil {
		return "", err
	}

	// privileged
	_, err = conn.Write([]byte("enable\n"))
	if err != nil {
		return "", err
	}
	err = conn.SkipUntil("Password: ")
	if err != nil {
		return "", err
	}
	_, err = conn.Write([]byte(password + "\n"))
	if err != nil {
		return "", err
	}
	err = conn.SkipUntil("#")
	if err != nil {
		return "", err
	}

	// multi-line command
	scanner := bufio.NewScanner(strings.NewReader(command))
	result := []byte{}
	for scanner.Scan() {
		_, err = conn.Write([]byte(scanner.Text() + "\n"))
		if err != nil {
			return "", err
		}
		data, err := conn.ReadBytes('#')
		if err != nil {
			return "", err
		}
		result = append(result, data...)
	}
	return string(result), nil
}
