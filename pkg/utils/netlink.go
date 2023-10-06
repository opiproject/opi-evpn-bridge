// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package utils has some utility functions and interfaces
package utils

import (
	"context"
	"net"

	"github.com/vishvananda/netlink"
)

// Netlink represents limited subset of functions from netlink package
type Netlink interface {
	LinkByName(context.Context, string) (netlink.Link, error)
	LinkModify(context.Context, netlink.Link) error
	LinkSetHardwareAddr(context.Context, netlink.Link, net.HardwareAddr) error
	AddrAdd(context.Context, netlink.Link, *netlink.Addr) error
	AddrDel(context.Context, netlink.Link, *netlink.Addr) error
	LinkAdd(context.Context, netlink.Link) error
	LinkDel(context.Context, netlink.Link) error
	LinkSetUp(context.Context, netlink.Link) error
	LinkSetDown(context.Context, netlink.Link) error
	LinkSetMaster(context.Context, netlink.Link, netlink.Link) error
	LinkSetNoMaster(context.Context, netlink.Link) error
	BridgeVlanAdd(context.Context, netlink.Link, uint16, bool, bool, bool, bool) error
	BridgeVlanDel(context.Context, netlink.Link, uint16, bool, bool, bool, bool) error
}

// NetlinkWrapper wrapper for netlink package
type NetlinkWrapper struct {
}

// build time check that struct implements interface
var _ Netlink = (*NetlinkWrapper)(nil)

// LinkByName is a wrapper for netlink.LinkByName
func (n *NetlinkWrapper) LinkByName(_ context.Context, name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

// LinkModify is a wrapper for netlink.LinkModify
func (n *NetlinkWrapper) LinkModify(_ context.Context, link netlink.Link) error {
	return netlink.LinkModify(link)
}

// LinkSetHardwareAddr is a wrapper for netlink.LinkSetHardwareAddr
func (n *NetlinkWrapper) LinkSetHardwareAddr(_ context.Context, link netlink.Link, hwaddr net.HardwareAddr) error {
	return netlink.LinkSetHardwareAddr(link, hwaddr)
}

// AddrAdd is a wrapper for netlink.AddrAdd
func (n *NetlinkWrapper) AddrAdd(_ context.Context, link netlink.Link, addr *netlink.Addr) error {
	return netlink.AddrAdd(link, addr)
}

// AddrDel is a wrapper for netlink.AddrDel
func (n *NetlinkWrapper) AddrDel(_ context.Context, link netlink.Link, addr *netlink.Addr) error {
	return netlink.AddrDel(link, addr)
}

// LinkAdd is a wrapper for netlink.LinkAdd
func (n *NetlinkWrapper) LinkAdd(_ context.Context, link netlink.Link) error {
	return netlink.LinkAdd(link)
}

// LinkDel is a wrapper for netlink.LinkDel
func (n *NetlinkWrapper) LinkDel(_ context.Context, link netlink.Link) error {
	return netlink.LinkDel(link)
}

// LinkSetUp is a wrapper for netlink.LinkSetUp
func (n *NetlinkWrapper) LinkSetUp(_ context.Context, link netlink.Link) error {
	return netlink.LinkSetUp(link)
}

// LinkSetDown is a wrapper for netlink.LinkSetDown
func (n *NetlinkWrapper) LinkSetDown(_ context.Context, link netlink.Link) error {
	return netlink.LinkSetDown(link)
}

// LinkSetMaster is a wrapper for netlink.LinkSetMaster
func (n *NetlinkWrapper) LinkSetMaster(_ context.Context, link, master netlink.Link) error {
	return netlink.LinkSetMaster(link, master)
}

// LinkSetNoMaster is a wrapper for netlink.LinkSetNoMaster
func (n *NetlinkWrapper) LinkSetNoMaster(_ context.Context, link netlink.Link) error {
	return netlink.LinkSetNoMaster(link)
}

// BridgeVlanAdd is a wrapper for netlink.BridgeVlanAdd
func (n *NetlinkWrapper) BridgeVlanAdd(_ context.Context, link netlink.Link, vid uint16, pvid, untagged, self, master bool) error {
	return netlink.BridgeVlanAdd(link, vid, pvid, untagged, self, master)
}

// BridgeVlanDel is a wrapper for netlink.BridgeVlanDel
func (n *NetlinkWrapper) BridgeVlanDel(_ context.Context, link netlink.Link, vid uint16, pvid, untagged, self, master bool) error {
	return netlink.BridgeVlanDel(link, vid, pvid, untagged, self, master)
}
