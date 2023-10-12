// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package utils contails useful helper functions
package utils

import (
	"context"
	"time"

	"github.com/ziutek/telnet"
)

const (
	network  = "tcp"
	password = "opi"
	address  = "localhost:2605"
	timeout  = 10 * time.Second
)

// TelnetDialAndCommunicate connects to telnet with password and runs command
func TelnetDialAndCommunicate(_ context.Context, command string) (string, error) {
	// new connection every time
	conn, err := telnet.DialTimeout(network, address, timeout)
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

	// command
	err = conn.SkipUntil(">")
	if err != nil {
		return "", err
	}
	_, err = conn.Write([]byte(command + "\n"))
	if err != nil {
		return "", err
	}

	// response
	data, err := conn.ReadBytes('>')
	if err != nil {
		return "", err
	}
	return string(data), nil
}
