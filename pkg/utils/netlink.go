// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package utils has some utility functions and interfaces
package utils

import (
	"context"
	"fmt"
	"net"
	"os/exec"

	"github.com/vishvananda/netlink"

	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Netlink represents limited subset of functions from netlink package
type Netlink interface {
	LinkByName(context.Context, string) (netlink.Link, error)
	LinkModify(context.Context, netlink.Link) error
	LinkSetHardwareAddr(context.Context, netlink.Link, net.HardwareAddr) error
	LinkSetVfHardwareAddr(context.Context, netlink.Link, int, net.HardwareAddr) error
	AddrAdd(context.Context, netlink.Link, *netlink.Addr) error
	AddrDel(context.Context, netlink.Link, *netlink.Addr) error
	AddrList(context.Context, netlink.Link, int) ([]netlink.Addr, error)
	LinkAdd(context.Context, netlink.Link) error
	LinkDel(context.Context, netlink.Link) error
	LinkSetUp(context.Context, netlink.Link) error
	LinkSetDown(context.Context, netlink.Link) error
	LinkSetMaster(context.Context, netlink.Link, netlink.Link) error
	LinkSetNoMaster(context.Context, netlink.Link) error
	LinkSetNsFd(context.Context, netlink.Link, int) error
	LinkSetName(context.Context, netlink.Link, string) error
	LinkSetVfRate(context.Context, netlink.Link, int, int, int) error
	LinkSetVfSpoofchk(context.Context, netlink.Link, int, bool) error
	LinkSetVfTrust(context.Context, netlink.Link, int, bool) error
	LinkSetVfState(context.Context, netlink.Link, int, uint32) error
	BridgeVlanAdd(context.Context, netlink.Link, uint16, bool, bool, bool, bool) error
	BridgeVlanDel(context.Context, netlink.Link, uint16, bool, bool, bool, bool) error
	LinkSetMTU(context.Context, netlink.Link, int) error
	BridgeFdbAdd(context.Context, string, string) error
	RouteAdd(context.Context, *netlink.Route) error
	RouteListFiltered(context.Context, int, *netlink.Route, uint64) ([]netlink.Route, error)
	RouteFlushTable(context.Context, string) error
	RouteListIPTable(context.Context, string) bool
	LinkSetBrNeighSuppress(context.Context, netlink.Link, bool) error
	ReadNeigh(context.Context, string) (string, error)
	ReadRoute(context.Context, string) (string, error)
	ReadFDB(context.Context) (string, error)
	RouteLookup(context.Context, string, string) (string, error)
}

// run function run the commands
func run(cmd []string, _ bool) (string, int) {
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

// NetlinkWrapper wrapper for netlink package
type NetlinkWrapper struct {
	tracer trace.Tracer
}

// NewNetlinkWrapper creates initialized instance of NetlinkWrapper
func NewNetlinkWrapper() *NetlinkWrapper {
	return NewNetlinkWrapperWithArgs(true)
}

// NewNetlinkWrapperWithArgs creates initialized instance of NetlinkWrapper
// based on passing arguments
func NewNetlinkWrapperWithArgs(enableTracer bool) *NetlinkWrapper {
	netlinkWrapper := &NetlinkWrapper{}
	netlinkWrapper.tracer = noop.NewTracerProvider().Tracer("")
	if enableTracer {
		netlinkWrapper.tracer = otel.Tracer("")
	}
	return netlinkWrapper
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

// LinkSetVfHardwareAddr is a wrapper for netlink.LinkSetVfHardwareAddr
func (n *NetlinkWrapper) LinkSetVfHardwareAddr(ctx context.Context, link netlink.Link, vf int, hwaddr net.HardwareAddr) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetVfHardwareAddrr")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetVfHardwareAddr(link, vf, hwaddr)
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

// AddrList is a wrapper for netlink.AddrList
func (n *NetlinkWrapper) AddrList(ctx context.Context, link netlink.Link, family int) ([]netlink.Addr, error) {
	_, childSpan := n.tracer.Start(ctx, "netlink.AddrList")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.AddrList(link, family)
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

// LinkSetMTU is a wrapper for netlink.LinkSetUp
func (n *NetlinkWrapper) LinkSetMTU(ctx context.Context, link netlink.Link, mtu int) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetMTU")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetMTU(link, mtu)
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

// LinkSetNsFd is a wrapper for netlink.LinkSetNsFd
func (n *NetlinkWrapper) LinkSetNsFd(ctx context.Context, link netlink.Link, fd int) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetNsFd")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetNsFd(link, fd)
}

// LinkSetName is a wrapper for netlink.LinkSetName
func (n *NetlinkWrapper) LinkSetName(ctx context.Context, link netlink.Link, name string) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetName")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetName(link, name)
}

// LinkSetVfRate is a wrapper for netlink.LinkSetVfRate
func (n *NetlinkWrapper) LinkSetVfRate(ctx context.Context, link netlink.Link, vf int, minRate int, maxRate int) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetVfRate")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetVfRate(link, vf, minRate, maxRate)
}

// LinkSetVfSpoofchk is a wrapper for netlink.LinkSetVfSpoofchk
func (n *NetlinkWrapper) LinkSetVfSpoofchk(ctx context.Context, link netlink.Link, vf int, check bool) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetVfSpoofchk")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetVfSpoofchk(link, vf, check)
}

// LinkSetVfTrust is a wrapper for netlink.LinkSetVfTrust
func (n *NetlinkWrapper) LinkSetVfTrust(ctx context.Context, link netlink.Link, vf int, state bool) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetVfTrust")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetVfTrust(link, vf, state)
}

// LinkSetVfState is a wrapper for netlink.LinkSetVfState
func (n *NetlinkWrapper) LinkSetVfState(ctx context.Context, link netlink.Link, vf int, state uint32) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetVfState")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetVfState(link, vf, state)
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

// RouteListFiltered is a wrapper for netlink.RouteListFiltered
func (n *NetlinkWrapper) RouteListFiltered(ctx context.Context, family int, route *netlink.Route, filter uint64) ([]netlink.Route, error) {
	_, childSpan := n.tracer.Start(ctx, "netlink.RouteListFiltered")
	//	link,_:=netlink.LinkByIndex(route.LinkIndex)
	childSpan.SetAttributes(attribute.String("route.LinkIndex", string(rune(route.LinkIndex))))
	defer childSpan.End()
	return netlink.RouteListFiltered(family, route, filter)
}

// RouteAdd is a wrapper for netlink.RouteAdd
func (n *NetlinkWrapper) RouteAdd(ctx context.Context, route *netlink.Route) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.RouteAdd")
	_, _ = netlink.LinkByIndex(route.LinkIndex)
	childSpan.SetAttributes(attribute.String("route.LinkIndex", string(rune(route.LinkIndex))))
	defer childSpan.End()
	return netlink.RouteAdd(route)
}

// RouteFlushTable is a wrapper for netlink.RouteFlushTable
func (n *NetlinkWrapper) RouteFlushTable(_ context.Context, routingTable string) error {
	_, err := run([]string{"ip", "route", "flush", "table", routingTable}, false)
	if err != 0 {
		return fmt.Errorf("lgm: Error in executing command ip route flush table %s", routingTable)
	}
	return nil
}

// RouteListIPTable is a wrapper for netlink.RouteListIPTable
func (n *NetlinkWrapper) RouteListIPTable(_ context.Context, vtip string) bool {
	_, err := run([]string{"ip", "route", "list", "exact", vtip, "table", "local"}, false)
	return err == 0
}

// BridgeFdbAdd is a wrapper for netlink.BridgeFdbAdd
func (n *NetlinkWrapper) BridgeFdbAdd(_ context.Context, link string, macAddress string) error {
	_, err := run([]string{"bridge", "fdb", "add", macAddress, "dev", link, "master", "static", "extern_learn"}, false)
	if err != 0 {
		return errors.New("failed to add fdb entry")
	}
	return nil
}

// ReadNeigh is a wrapper for netlink.ReadNeigh
func (n *NetlinkWrapper) ReadNeigh(_ context.Context, link string) (string, error) {
	var out string
	var err int
	if link == "" {
		out, err = run([]string{"ip", "-j", "-d", "neighbor", "show"}, false)
	} else {
		out, err = run([]string{"ip", "-j", "-d", "neighbor", "show", "vrf", link}, false)
	}
	if err != 0 {
		return "", errors.New("failed routelookup")
	}
	return out, nil
}

// ReadRoute is a wrapper for netlink.ReadRoute
func (n *NetlinkWrapper) ReadRoute(_ context.Context, table string) (string, error) {
	out, err := run([]string{"ip", "-j", "-d", "route", "show", "table", table}, false)
	if err != 0 {
		return "", errors.New("failed to read route")
	}
	return out, nil
}

// ReadFDB is a wrapper for netlink.ReadFDB
func (n *NetlinkWrapper) ReadFDB(_ context.Context) (string, error) {
	out, err := run([]string{"bridge", "-d", "-j", "fdb", "show", "br", "br-tenant", "dynamic"}, false)
	if err != 0 {
		return "", errors.New("failed to read fdb")
	}
	return out, nil
}

// RouteLookup is a wrapper for netlink.RouteLookup
func (n *NetlinkWrapper) RouteLookup(_ context.Context, dst string, link string) (string, error) {
	var out string
	var err int
	if link == "" {
		out, err = run([]string{"ip", "-j", "route", "get", dst, "fibmatch"}, false)
	} else {
		out, err = run([]string{"ip", "-j", "route", "get", dst, "vrf", link, "fibmatch"}, false)
	}
	if err != 0 {
		return "", errors.New("failed routelookup")
	}
	return out, nil
}

// LinkSetBrNeighSuppress is a wrapper for netlink.LinkSetBrNeighSuppress
func (n *NetlinkWrapper) LinkSetBrNeighSuppress(ctx context.Context, link netlink.Link, neighSuppress bool) error {
	_, childSpan := n.tracer.Start(ctx, "netlink.LinkSetBrNeighSuppress")
	childSpan.SetAttributes(attribute.String("link.name", link.Attrs().Name))
	defer childSpan.End()
	return netlink.LinkSetBrNeighSuppress(link, neighSuppress)
}
