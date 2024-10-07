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
	notifyDBCompChanges[RouteKey, *RouteStruct](latestRoutes, routes, ROUTE, routeOperations)
	notifyDBCompChanges[NexthopKey, *NexthopStruct](latestNexthop, nexthops, NEXTHOP, nexthopOperations)
	notifyDBCompChanges[FdbKey, *FdbEntryStruct](latestFDB, fDB, FDB, fdbOperations)
	notifyDBCompChanges[L2NexthopKey, *L2NexthopStruct](latestL2Nexthop, l2Nexthops, L2NEXTHOP, l2NexthopOperations)
}

// Filterable method for each type
type Filterable interface {
	ShouldFilter() bool
}

// applyInstallFilters install the filters
func applyInstallFilters() {
	filterMap(latestRoutes, func(r *RouteStruct) bool { return r.installFilterRoute() })
	filterMap(latestNexthop, func(n *NexthopStruct) bool { return n.installFilterNH() })
	filterMap(latestFDB, func(f *FdbEntryStruct) bool { return f.installFilterFDB() })
	filterMap(latestL2Nexthop, func(l *L2NexthopStruct) bool { return l.installFilterL2N() })
}

// filterMap install the filters
func filterMap[K comparable, V Filterable](m map[K]V, shouldFilter func(V) bool) {
	for key, value := range m {
		if !shouldFilter(value) {
			delete(m, key)
		}
	}
}

// ShouldFilter method for each type
func (route *RouteStruct) ShouldFilter() bool {
	return route.installFilterRoute()
}

// ShouldFilter method for each type
func (nexthop *NexthopStruct) ShouldFilter() bool {
	return nexthop.installFilterNH()
}

// ShouldFilter method for each type
func (fdb *FdbEntryStruct) ShouldFilter() bool {
	return fdb.installFilterFDB()
}

// ShouldFilter method for each type
func (l *L2NexthopStruct) ShouldFilter() bool {
	return l.installFilterL2N()
}

// Annotatable interface
type Annotatable interface {
	annotate()
}

// annotateMap  annonates the latest db map updates
func annotateMap[K comparable, V Annotatable](m map[K]V) {
	for key, value := range m {
		value.annotate()
		m[key] = value
	}
}

// annotateDBEntries  annonates the latest db updates
func annotateDBEntries() {
	annotateMap(latestNexthop)
	annotateMap(latestRoutes)
	annotateMap(latestFDB)
	annotateMap(latestL2Nexthop)
}

// readLatestNetlinkState reads the latest netlink state
func readLatestNetlinkState() {
	grdVrf, err := infradb.GetVrf("//network.opiproject.org/vrfs/GRD")
	if err == nil {
		readNeighbors(grdVrf)
		readRoutes(grdVrf)
		vrfs, _ := infradb.GetAllVrfs()
		for _, v := range vrfs {
			if v.Name != grdVrf.Name {
				readNeighbors(v) // viswanantha library
				readRoutes(v)    // Viswantha library
			}
		}
		m := readFDB()
		for i := 0; i < len(m); i++ {
			m[i].addFdbEntry()
		}
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

// notifyUpdates notifies the db updates
func notifyUpdates[K comparable, V any](items map[K]V, deletionType string) {
	for _, item := range items {
		notifyAddDel(item, deletionType)
	}
}

// Usage

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
	notifyUpdates(routes, RouteDeleted)
	notifyUpdates(nexthops, NexthopDeleted)
	notifyUpdates(fDB, FdbEntryDeleted)
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

	grdDefaultRoute = config.GlobalConfig.Netlink.GrdDefaultRoute
	enableEcmp = config.GlobalConfig.Netlink.EnableEcmp

	if !nlEnabled {
		log.Printf("netlink: netlink_monitor disabled")
		return
	}
	for i := 0; i < len(config.GlobalConfig.Interfaces.PhyPorts); i++ {
		phyPorts[config.GlobalConfig.Interfaces.PhyPorts[i].Rep] = config.GlobalConfig.Interfaces.PhyPorts[i].Vsi
	}
	getlink()
	ctx = context.Background()
	nlink = utils.NewNetlinkWrapperWithArgs(config.GlobalConfig.Tracer)
	stopMonitoring.Store(false)
	go monitorNetlink() // monitor Thread started
}

// DeInitialize function handles stops functionality
func DeInitialize() {
	// stopMonitoring = true
	stopMonitoring.Store(true)
}
