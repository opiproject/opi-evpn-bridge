// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package utils contails useful helper functions
package utils

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ziutek/telnet"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const (
	network        = "tcp"
	password       = "opi"
	defaultAddress = "localhost"
	timeout        = 10 * time.Second
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

// Frr represents limited subset of functions from Frr package
type Frr interface {
	TelnetDialAndCommunicate(ctx context.Context, command string, port int) (string, error)
	FrrZebraCmd(ctx context.Context, command string) (string, error)
	FrrBgpCmd(ctx context.Context, command string) (string, error)
	Save(context.Context) error
	Password(conn *telnet.Conn, delim string) error
	EnterPrivileged(conn *telnet.Conn) error
	ExitPrivileged(conn *telnet.Conn) error
}

// FrrWrapper wrapper for Frr package
type FrrWrapper struct {
	address string
	tracer  trace.Tracer
}

// NewFrrWrapper creates initialized instance of FrrWrapper with default address
func NewFrrWrapper() *FrrWrapper {
	return NewFrrWrapperWithArgs(defaultAddress, true)
}

// NewFrrWrapperWithArgs creates initialized instance of FrrWrapper
func NewFrrWrapperWithArgs(address string, enableTracer bool) *FrrWrapper {
	frrWrapper := &FrrWrapper{address: address}
	frrWrapper.tracer = noop.NewTracerProvider().Tracer("")
	if enableTracer {
		// default tracer name is good for now
		frrWrapper.tracer = otel.Tracer("")
	}
	return frrWrapper
}

// build time check that struct implements interface
var _ Frr = (*FrrWrapper)(nil)

// Password handles password sending
func (n *FrrWrapper) Password(conn *telnet.Conn, delim string) error {
	err := conn.SkipUntil("Password: ")
	if err != nil {
		return err
	}
	_, err = conn.Write([]byte(password + "\n"))
	if err != nil {
		return err
	}
	return conn.SkipUntil(delim)
}

// EnterPrivileged turns on privileged mode command
func (n *FrrWrapper) EnterPrivileged(conn *telnet.Conn) error {
	_, err := conn.Write([]byte("enable\n"))
	if err != nil {
		return err
	}
	return n.Password(conn, "#")
}

// ExitPrivileged turns off privileged mode command
func (n *FrrWrapper) ExitPrivileged(conn *telnet.Conn) error {
	_, err := conn.Write([]byte("disable\n"))
	if err != nil {
		return err
	}
	return conn.SkipUntil(">")
}

// FrrZebraCmd connects to Zebra telnet with password and runs command
func (n *FrrWrapper) FrrZebraCmd(ctx context.Context, command string) (string, error) {
	// ports defined here https://docs.frrouting.org/en/latest/setup.html#services
	return n.TelnetDialAndCommunicate(ctx, command, zebra)
}

// FrrBgpCmd connects to Bgp telnet with password and runs command
func (n *FrrWrapper) FrrBgpCmd(ctx context.Context, command string) (string, error) {
	// ports defined here https://docs.frrouting.org/en/latest/setup.html#services
	return n.TelnetDialAndCommunicate(ctx, command, bgpd)
}

// Save command save the current config to /etc/frr/frr.conf
func (n *FrrWrapper) Save(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "vtysh", "-c", "write")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to save FRR config: %v, output: %s", err, output)
	}
	return nil
}

// MultiLineCmd breaks command by lines, sends each and waits for output and returns combined output
func (n *FrrWrapper) MultiLineCmd(conn *telnet.Conn, command string) (string, error) {
	// multi-line command
	scanner := bufio.NewScanner(strings.NewReader(command))
	result := []byte{}
	for scanner.Scan() {
		_, err := conn.Write([]byte(scanner.Text() + "\n"))
		if err != nil {
			return string(result), err
		}
		data, err := conn.ReadBytes('#')
		if err != nil {
			return string(result), err
		}
		result = append(result, data...)
	}
	return string(result), nil
}

// TelnetDialAndCommunicate connects to telnet with password and runs command
func (n *FrrWrapper) TelnetDialAndCommunicate(ctx context.Context, command string, port int) (string, error) {
	_, childSpan := n.tracer.Start(ctx, "frr.Command")
	defer childSpan.End()

	if childSpan.IsRecording() {
		childSpan.SetAttributes(
			attribute.Int("frr.port", port),
			attribute.String("frr.name", command),
			attribute.String("frr.address", n.address),
			attribute.String("frr.network", network),
		)
	}

	// new connection every time
	conn, err := telnet.DialTimeout(network, fmt.Sprintf("%s:%d", n.address, port), timeout)
	if err != nil {
		return "", err
	}
	defer func(t *telnet.Conn) { _ = t.Close() }(conn)

	conn.SetUnixWriteMode(true)

	err = conn.SetWriteDeadline(time.Now().Add(timeout))
	if err != nil {
		return "", err
	}

	err = n.Password(conn, ">")
	if err != nil {
		return "", err
	}

	err = n.EnterPrivileged(conn)
	if err != nil {
		return "", err
	}

	return n.MultiLineCmd(conn, command)
}
