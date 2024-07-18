// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2023 Nordix Foundation.

// Package utils has some utility functions and interfaces
package utils

import (
	"context"
	"log"
	"net"
	"os/exec"
	"regexp"

	"github.com/vishvananda/netlink"
	"go.einride.tech/aip/fieldmask"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ApplyMaskToStoredPbObject updates the stored PB object with the one provided
// in the update grpc request based on the provided field mask
func ApplyMaskToStoredPbObject[T proto.Message](updateMask *fieldmaskpb.FieldMask, dst, src T) {
	fieldmask.Update(updateMask, dst, src)
}

// Run function run the commands
func Run(cmd []string, _ bool) (string, int) {
	var out []byte
	var err error
	out, err = exec.Command(cmd[0], cmd[1:]...).CombinedOutput() //nolint:gosec
	if err != nil {
		/*if flag {
			// panic(fmt.Sprintf("Command %s': exit code %s;", out, err.Error()))
		}
		// fmt.Printf("Command %s': exit code %s;\n", out, err)*/
		return "Error in running command", -1
	}
	output := string(out)
	return output, 0
}

// ValidateMacAddress validates if a passing MAC address
// has the right format
func ValidateMacAddress(b []byte) error {
	macPattern := "([0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2})"
	_, err := regexp.MatchString(macPattern, string(b))
	if err != nil {
		return err
	}
	return nil
}

// GetIPAddress gets the ip address from link
func GetIPAddress(dev string) net.IPNet {
	nlink := NewNetlinkWrapper()
	ctx := context.Background()
	link, err := nlink.LinkByName(ctx, dev)
	if err != nil {
		log.Printf("Error in LinkByName %+v\n", err)
		return net.IPNet{
			IP: net.ParseIP("0.0.0.0"),
		}
	}

	addrs, err := nlink.AddrList(ctx, link, netlink.FAMILY_V4) // ip address show
	if err != nil {
		log.Printf("Error in AddrList\n")
		return net.IPNet{
			IP: net.ParseIP("0.0.0.0"),
		}
	}
	var address = &net.IPNet{
		IP:   net.IPv4(127, 0, 0, 0),
		Mask: net.CIDRMask(8, 32)}
	var addr = &netlink.Addr{IPNet: address}
	var validIps []netlink.Addr
	for index := 0; index < len(addrs); index++ {
		if !addr.Equal(addrs[index]) {
			validIps = append(validIps, addrs[index])
		}
	}
	return *validIps[0].IPNet
}
