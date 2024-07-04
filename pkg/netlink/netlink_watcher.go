// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package netlink handles the netlink related functionality
package netlink

import (
	"context"
	"log"

	"time"

	vn "github.com/vishvananda/netlink"

	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

// DeleteLatestDB deletes the latest db snap
func DeleteLatestDB() {
	latestRoutes = make(map[routeKey]*RouteStruct)
	latestNeighbors = make(map[neighKey]neighStruct)
	latestNexthop = make(map[nexthopKey]*NexthopStruct)
	latestFDB = make(map[fdbKey]*FdbEntryStruct)
	latestL2Nexthop = make(map[l2NexthopKey]*L2NexthopStruct)
}

// resyncWithKernel fun resyncs with kernal db
func resyncWithKernel() {
	// Build a new DB snapshot from netlink and other sources
	readLatestNetlinkState()
	// Annotate the latest DB entries
	annotateDBEntries()
	// Filter the latest DB to retain only entries to be installed
	applyInstallFilters()
	// Compute changes between current and latest DB versions and inform subscribers about the changes
	notifyDBChanges()
	routes = latestRoutes
	Nexthops = latestNexthop
	Neighbors = latestNeighbors
	fDB = latestFDB
	l2Nexthops = latestL2Nexthop
	DeleteLatestDB()
}

// monitorNetlink moniters the netlink
func monitorNetlink() {
	for !stopMonitoring.Load().(bool) {
		resyncWithKernel()
		time.Sleep(time.Duration(pollInterval) * time.Second)
	}
	log.Printf("netlink: Stopped periodic polling. Waiting for Infra DB cleanup to finish")
	time.Sleep(2 * time.Second)
	log.Printf("netlink: One final netlink poll to identify what's still left.")
	// Inform subscribers to delete configuration for any still remaining Netlink DB objects.
	log.Printf("netlink: Delete any residual objects in DB")
	for _, r := range routes {
		notifyAddDel(r, RouteDeleted)
	}

	for _, nexthop := range Nexthops {
		notifyAddDel(nexthop, NexthopDeleted)
	}

	for _, m := range fDB {
		notifyAddDel(m, FdbEntryDeleted)
	}
	log.Printf("netlink: DB cleanup completed.")
}

// getlink get the link
func getlink() {
	links, err := vn.LinkList()
	if err != nil {
		log.Fatal("netlink:", err)
	}
	for i := 0; i < len(links); i++ {
		linkTable = append(linkTable, links[i])
		NameIndex[links[i].Attrs().Index] = links[i].Attrs().Name
		switch links[i].Type() {
		case "vrf":
			vrfList = append(vrfList, links[i])
		case "device":
			deviceList = append(deviceList, links[i])
		case "vlan":
			vlanList = append(vlanList, links[i])
		case "bridge":
			bridgeList = append(bridgeList, links[i])
		case "vxlan":
			vxlanList = append(vxlanList, links[i])
		default:
		}
		linkList = append(linkList, links[i])
	}
}

// Initialize function intializes config
func Initialize() {
	pollInterval = config.GlobalConfig.Netlink.PollInterval
	log.Printf("netlink: poll interval: %v", pollInterval)
	nlEnabled := config.GlobalConfig.Netlink.Enabled
	if !nlEnabled {
		log.Printf("netlink: netlink_monitor disabled")
		return
	}
	for i := 0; i < len(config.GlobalConfig.Netlink.PhyPorts); i++ {
		phyPorts[config.GlobalConfig.Netlink.PhyPorts[i].Name] = config.GlobalConfig.Netlink.PhyPorts[i].Vsi
	}
	getlink()
	ctx = context.Background()
	nlink = utils.NewNetlinkWrapperWithArgs(config.GlobalConfig.Tracer)
	// stopMonitoring = false
	stopMonitoring.Store(false)
	go monitorNetlink() // monitor Thread started
}

// DeInitialize function handles stops functionality
func DeInitialize() {
	// stopMonitoring = true
	stopMonitoring.Store(true)
}
