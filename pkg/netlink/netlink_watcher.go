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

// dboperations interface
type dboperations interface {
	annotate()
	filter() bool
}

type genericDB[K comparable, V dboperations] struct {
	compDB map[K]V
}

func newGenericDB[K comparable, V dboperations](data map[K]V) *genericDB[K, V] {
	return &genericDB[K, V]{compDB: data}
}

func (db *genericDB[K, V]) annotate() {
	for key, value := range db.compDB {
		value.annotate()
		db.compDB[key] = value
	}
}

func (db *genericDB[K, V]) filter() {
	for key, value := range db.compDB {
		if !value.filter() {
			delete(db.compDB, key)
		}
	}
}

// annotateAndFilterDB  annonates and filters the latest db updates
func annotateAndFilterDB() {
	latestNexthopDB := newGenericDB(latestNexthop)
	latestNexthopDB.annotate()
	latestNexthopDB.filter()

	latestRoutesDB := newGenericDB(latestRoutes)
	latestRoutesDB.annotate()
	latestRoutesDB.filter()

	latestFdbDB := newGenericDB(latestFDB)
	latestFdbDB.annotate()
	latestFdbDB.filter()

	latestL2NexthopDB := newGenericDB(latestL2Nexthop)
	latestL2NexthopDB.annotate()
	latestL2NexthopDB.filter()

	latestNexthop = latestNexthopDB.compDB
	latestRoutes = latestRoutesDB.compDB
	latestFDB = latestFdbDB.compDB
	latestL2Nexthop = latestL2NexthopDB.compDB
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
		m[i].addFdbEntry()
	}
	dumpDBs()
}

// resyncWithKernel fun resyncs with kernal db
func resyncWithKernel() {
	// Build a new DB snapshot from netlink and other sources
	readLatestNetlinkState()
	// Annotate and filter the latest DB entries
	annotateAndFilterDB()
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
