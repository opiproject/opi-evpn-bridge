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
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

// deleteLatestDB deletes the latest db snap
func deleteLatestDB() {
	latestRoutes = make(map[RouteKey]*RouteStruct)
	latestNeighbors = make(map[NeighKey]NeighStruct)
	latestNexthop = make(map[NexthopKey]*NexthopStruct)
	latestFDB = make(map[FdbKey]*FdbEntryStruct)
	latestL2Nexthop = make(map[L2NexthopKey]*L2NexthopStruct)
}

// notifyDBChanges notify the database changes
func notifyDBChanges() {
	processAndnotify[RouteKey, *RouteStruct](latestRoutes, routes, ROUTE, routeOperations)
	processAndnotify[NexthopKey, *NexthopStruct](latestNexthop, nexthops, NEXTHOP, nexthopOperations)
	processAndnotify[FdbKey, *FdbEntryStruct](latestFDB, fDB, FDB, fdbOperations)
	processAndnotify[L2NexthopKey, *L2NexthopStruct](latestL2Nexthop, l2Nexthops, L2NEXTHOP, l2NexthopOperations)
}

// nolint
func applyInstallFilters() {
	for K, r := range latestRoutes {
		if !installFilterRoute(r) {
			// Remove route from its nexthop(s)
			delete(latestRoutes, K)
		}
	}

	for k, nexthop := range latestNexthop {
		if !installFilterNH(nexthop) {
			delete(latestNexthop, k)
		}
	}

	for k, m := range latestFDB {
		if !installFilterFDB(m) {
			delete(latestFDB, k)
		}
	}
	for k, L2 := range latestL2Nexthop {
		if !installFilterL2N(L2) {
			delete(latestL2Nexthop, k)
		}
	}
}

// annotateDBEntries annonates the database entries
func annotateDBEntries() {
	for _, nexthop := range latestNexthop {
		nexthop.annotate()
		latestNexthop[nexthop.Key] = nexthop
	}
	for _, r := range latestRoutes {
		r.annotate()
		latestRoutes[r.Key] = r
	}

	for _, m := range latestFDB {
		m.annotate()
		latestFDB[m.Key] = m
	}
	for _, l2n := range latestL2Nexthop {
		l2n.annotate()
		latestL2Nexthop[l2n.Key] = l2n
	}
}

// readLatestNetlinkState reads the latest netlink state
func readLatestNetlinkState() {
	vrfs, _ := infradb.GetAllVrfs()
	for _, v := range vrfs {
		readNeighbors(v) // viswanantha library
		readRoutes(v)    // Viswantha library
	}
	m := readFDB()
	for i := 0; i < len(m); i++ {
		addFdbEntry(m[i])
	}
	dumpDBs()
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
	nexthops = latestNexthop
	fDB = latestFDB
	l2Nexthops = latestL2Nexthop
	deleteLatestDB()
}

// monitorNetlink moniters the netlink
func monitorNetlink() {
	for !stopMonitoring.Load() {
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

	for _, nexthop := range nexthops {
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
		nameIndex[links[i].Attrs().Index] = links[i].Attrs().Name
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
