// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package utils has some utility functions and interfaces
package utils

import (
	"context"
	"net"

	"github.com/vishvananda/netlink"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	tracer trace.Tracer
}

// NewNetlinkWrapper creates initialized instance of NetlinkWrapper
func NewNetlinkWrapper() *NetlinkWrapper {
	return &NetlinkWrapper{tracer: otel.Tracer("")}
}

// build time check that struct implements interface
var _ Netlink = (*NetlinkWrapper)(nil)

// LinkByName is a wrapper for netlink.LinkByName
func (n *NetlinkWrapper) LinkByName(ctx context.Context, name string) (netlink.Link, error) {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkByName")
	childSpan.SetAttributes(attribute.String("link.name", name))
	defer childSpan.End()
	return netlink.LinkByName(name)
}

// LinkModify is a wrapper for netlink.LinkModify
func (n *NetlinkWrapper) LinkModify(ctx context.Context, link netlink.Link) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkModify")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkModify(link)
}

// LinkSetHardwareAddr is a wrapper for netlink.LinkSetHardwareAddr
func (n *NetlinkWrapper) LinkSetHardwareAddr(ctx context.Context, link netlink.Link, hwaddr net.HardwareAddr) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetHardwareAddr")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetHardwareAddr(link, hwaddr)
}

// AddrAdd is a wrapper for netlink.AddrAdd
func (n *NetlinkWrapper) AddrAdd(ctx context.Context, link netlink.Link, addr *netlink.Addr) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.AddrAdd")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.AddrAdd(link, addr)
}

// AddrDel is a wrapper for netlink.AddrDel
func (n *NetlinkWrapper) AddrDel(ctx context.Context, link netlink.Link, addr *netlink.Addr) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.AddrDel")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.AddrDel(link, addr)
}

// LinkAdd is a wrapper for netlink.LinkAdd
func (n *NetlinkWrapper) LinkAdd(ctx context.Context, link netlink.Link) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkAdd")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkAdd(link)
}

// LinkDel is a wrapper for netlink.LinkDel
func (n *NetlinkWrapper) LinkDel(ctx context.Context, link netlink.Link) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkDel")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkDel(link)
}

// LinkSetUp is a wrapper for netlink.LinkSetUp
func (n *NetlinkWrapper) LinkSetUp(ctx context.Context, link netlink.Link) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetUp")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetUp(link)
}

// LinkSetDown is a wrapper for netlink.LinkSetDown
func (n *NetlinkWrapper) LinkSetDown(ctx context.Context, link netlink.Link) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetDown")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetDown(link)
}

// LinkSetMaster is a wrapper for netlink.LinkSetMaster
func (n *NetlinkWrapper) LinkSetMaster(ctx context.Context, link, master netlink.Link) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetMaster")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetMaster(link, master)
}

// LinkSetNoMaster is a wrapper for netlink.LinkSetNoMaster
func (n *NetlinkWrapper) LinkSetNoMaster(ctx context.Context, link netlink.Link) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetNoMaster")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetNoMaster(link)
}

// BridgeVlanAdd is a wrapper for netlink.BridgeVlanAdd
func (n *NetlinkWrapper) BridgeVlanAdd(ctx context.Context, link netlink.Link, vid uint16, pvid, untagged, self, master bool) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.BridgeVlanAdd")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.BridgeVlanAdd(link, vid, pvid, untagged, self, master)
}

// BridgeVlanDel is a wrapper for netlink.BridgeVlanDel
func (n *NetlinkWrapper) BridgeVlanDel(ctx context.Context, link netlink.Link, vid uint16, pvid, untagged, self, master bool) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.BridgeVlanDel")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.BridgeVlanDel(link, vid, pvid, untagged, self, master)
}
