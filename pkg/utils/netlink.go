// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package utils has some utility functions and interfaces
package utils

import (
	"net"

	"github.com/vishvananda/netlink"
)

// Netlink represents limited subset of functions from netlink package
type Netlink interface {
	LinkByName(string) (netlink.Link, error)
	LinkModify(netlink.Link) error
	LinkSetHardwareAddr(netlink.Link, net.HardwareAddr) error
	AddrAdd(netlink.Link, *netlink.Addr) error
	AddrDel(netlink.Link, *netlink.Addr) error
	LinkAdd(netlink.Link) error
	LinkDel(netlink.Link) error
	LinkSetUp(netlink.Link) error
	LinkSetDown(netlink.Link) error
	LinkSetMaster(netlink.Link, netlink.Link) error
	LinkSetNoMaster(netlink.Link) error
	BridgeVlanAdd(netlink.Link, uint16, bool, bool, bool, bool) error
	BridgeVlanDel(netlink.Link, uint16, bool, bool, bool, bool) error
}

// NetlinkWrapper wrapper for netlink package
type NetlinkWrapper struct {
}

// LinkByName is a wrapper for netlink.LinkByName
func (n *NetlinkWrapper) LinkByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

// LinkModify is a wrapper for netlink.LinkModify
func (n *NetlinkWrapper) LinkModify(link netlink.Link) error {
	return netlink.LinkModify(link)
}

// LinkSetHardwareAddr is a wrapper for netlink.LinkSetHardwareAddr
func (n *NetlinkWrapper) LinkSetHardwareAddr(link netlink.Link, hwaddr net.HardwareAddr) error {
	return netlink.LinkSetHardwareAddr(link, hwaddr)
}

// AddrAdd is a wrapper for netlink.AddrAdd
func (n *NetlinkWrapper) AddrAdd(link netlink.Link, addr *netlink.Addr) error {
	return netlink.AddrAdd(link, addr)
}

// AddrDel is a wrapper for netlink.AddrDel
func (n *NetlinkWrapper) AddrDel(link netlink.Link, addr *netlink.Addr) error {
	return netlink.AddrDel(link, addr)
}

// LinkAdd is a wrapper for netlink.LinkAdd
func (n *NetlinkWrapper) LinkAdd(link netlink.Link) error {
	return netlink.LinkAdd(link)
}

// LinkDel is a wrapper for netlink.LinkDel
func (n *NetlinkWrapper) LinkDel(link netlink.Link) error {
	return netlink.LinkDel(link)
}

// LinkSetUp is a wrapper for netlink.LinkSetUp
func (n *NetlinkWrapper) LinkSetUp(link netlink.Link) error {
	return netlink.LinkSetUp(link)
}

// LinkSetDown is a wrapper for netlink.LinkSetDown
func (n *NetlinkWrapper) LinkSetDown(link netlink.Link) error {
	return netlink.LinkSetDown(link)
}

// LinkSetMaster is a wrapper for netlink.LinkSetMaster
func (n *NetlinkWrapper) LinkSetMaster(link, master netlink.Link) error {
	return netlink.LinkSetMaster(link, master)
}

// LinkSetNoMaster is a wrapper for netlink.LinkSetNoMaster
func (n *NetlinkWrapper) LinkSetNoMaster(link netlink.Link) error {
	return netlink.LinkSetNoMaster(link)
}

// BridgeVlanAdd is a wrapper for netlink.BridgeVlanAdd
func (n *NetlinkWrapper) BridgeVlanAdd(link netlink.Link, vid uint16, pvid, untagged, self, master bool) error {
	return netlink.BridgeVlanAdd(link, vid, pvid, untagged, self, master)
}

// BridgeVlanDel is a wrapper for netlink.BridgeVlanDel
func (n *NetlinkWrapper) BridgeVlanDel(link netlink.Link, vid uint16, pvid, untagged, self, master bool) error {
	return netlink.BridgeVlanDel(link, vid, pvid, untagged, self, master)
}
