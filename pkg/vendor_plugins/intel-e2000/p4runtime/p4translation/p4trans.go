// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package p4translation handles the intel e2000 fast path configuration
package p4translation

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	nm "github.com/opiproject/opi-evpn-bridge/pkg/netlink"
	eb "github.com/opiproject/opi-evpn-bridge/pkg/netlink/eventbus"
	p4client "github.com/opiproject/opi-evpn-bridge/pkg/vendor_plugins/intel-e2000/p4runtime/p4driverapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// L3 of type l3 decoder
var L3 L3Decoder

// Vxlan var vxlan of type vxlan decoder
var Vxlan VxlanDecoder

// Pod var pod of type pod decoder
var Pod PodDecoder

// ModuleipuHandler var empty struct of type module handler
type ModuleipuHandler struct{}

// isValidMAC checks if mac is valid
func isValidMAC(mac string) bool {
	macPattern := `^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`

	match, _ := regexp.MatchString(macPattern, mac)
	return match
}

// getMac get the mac from interface
func getMac(dev string) string {
	cmd := exec.Command("ip", "-d", "-j", "link", "show", dev)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("intel-e2000: Error running command: %v\n", err)
		return ""
	}

	var links []struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(out, &links); err != nil {
		log.Printf("intel-e2000: Error unmarshaling JSON: %v\n", err)
		return ""
	}

	if len(links) > 0 {
		mac := links[0].Address
		return mac
	}

	return ""
}

// vportFromMac get the vport from the mac
func vportFromMac(mac string) int {
	mbyte := strings.Split(mac, ":")
	if len(mbyte) < 5 {
		return -1
	}
	byte0, _ := strconv.ParseInt(mbyte[0], 16, 64)
	byte1, _ := strconv.ParseInt(mbyte[1], 16, 64)

	return int(byte0<<8 + byte1)
}

// idsOf  get the mac vsi from nexthop id
func idsOf(value string) (string, string, error) {
	if isValidMAC(value) {
		return strconv.Itoa(vportFromMac(value)), value, nil
	}

	mac := getMac(value)
	vsi := vportFromMac(mac)
	if vsi == -1 {
		return "", "", fmt.Errorf("failed to get id")
	}
	return strconv.Itoa(vsi), mac, nil
}

var (
	// defaultAddr default address
	defaultAddr = "127.0.0.1:9559"

	// Conn default grpc connection
	Conn *grpc.ClientConn
)

// startSubscriber  set the subscriber handlers
func startSubscriber(eventBus *eb.EventBus, eventType string) {
	subscriber := eventBus.Subscribe(eventType)

	go func() {
		for {
			select {
			case event := <-subscriber.Ch:
				log.Printf("intel-e2000: Subscriber for %s received event: %s\n", eventType, event)
				switch eventType {
				case "route_added":
					handleRouteAdded(event)
				case "route_updated":
					handleRouteUpdated(event)
				case "route_deleted":
					handleRouteDeleted(event)
				case "nexthop_added":
					handleNexthopAdded(event)
				case "nexthop_updated":
					handleNexthopUpdated(event)
				case "nexthop_deleted":
					handleNexthopDeleted(event)
				case "fdb_entry_added":
					handleFbdEntryAdded(event)
				case "fdb_entry_updated":
					handleFbdEntryUpdated(event)
				case "fdb_entry_deleted":
					handleFbdEntryDeleted(event)
				case "l2_nexthop_added":
					handleL2NexthopAdded(event)
				case "l2_nexthop_updated":
					handleL2NexthopUpdated(event)
				case "l2_nexthop_deleted":
					handleL2NexthopDeleted(event)
				}
			case <-subscriber.Quit:
				return
			}
		}
	}()
}

// handleRouteAdded  handles the added route
func handleRouteAdded(route interface{}) {
	var entries []interface{}
	routeData, _ := route.(nm.RouteStruct)
	entries = L3.translateAddedRoute(routeData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Printf("intel-e2000: Entry is not of type p4client.TableEntry:- %v\n", e)
		}
	}
}

// handleRouteUpdated  handles the updated route
func handleRouteUpdated(route interface{}) {
	var entries []interface{}
	routeData, _ := route.(nm.RouteStruct)
	entries = L3.translateDeletedRoute(routeData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			err := p4client.DelEntry(e)
			if err != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, err)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = append(entries, L3.translateAddedRoute(routeData))
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// handleRouteDeleted  handles the deleted route
func handleRouteDeleted(route interface{}) {
	var entries []interface{}
	routeData, _ := route.(nm.RouteStruct)
	entries = L3.translateDeletedRoute(routeData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// handleNexthopAdded  handles the added nexthop
func handleNexthopAdded(nexthop interface{}) {
	var entries []interface{}
	nexthopData, _ := nexthop.(nm.NexthopStruct)
	entries = L3.translateAddedNexthop(nexthopData)

	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Vxlan.translateAddedNexthop(nexthopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// handleNexthopUpdated  handles the updated nexthop
func handleNexthopUpdated(nexthop interface{}) {
	var entries []interface{}
	nexthopData, _ := nexthop.(nm.NexthopStruct)
	entries = L3.translateDeletedNexthop(nexthopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Vxlan.translateDeletedNexthop(nexthopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = L3.translateAddedNexthop(nexthopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Vxlan.translateAddedNexthop(nexthopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// handleNexthopDeleted  handles the deleted nexthop
func handleNexthopDeleted(nexthop interface{}) {
	var entries []interface{}
	nexthopData, _ := nexthop.(nm.NexthopStruct)
	entries = L3.translateDeletedNexthop(nexthopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Vxlan.translateDeletedNexthop(nexthopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// handleFbdEntryAdded  handles the added fdb entry
func handleFbdEntryAdded(fbdEntry interface{}) {
	var entries []interface{}
	fbdEntryData, _ := fbdEntry.(nm.FdbEntryStruct)
	entries = Vxlan.translateAddedFdb(fbdEntryData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Pod.translateAddedFdb(fbdEntryData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// handleFbdEntryUpdated  handles the updated fdb entry
func handleFbdEntryUpdated(fdbEntry interface{}) {
	var entries []interface{}
	fbdEntryData, _ := fdbEntry.(nm.FdbEntryStruct)
	entries = Vxlan.translateDeletedFdb(fbdEntryData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Pod.translateDeletedFdb(fbdEntryData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}

	entries = Vxlan.translateAddedFdb(fbdEntryData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Pod.translateAddedFdb(fbdEntryData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// handleFbdEntryDeleted  handles the deleted fdb entry
func handleFbdEntryDeleted(fdbEntry interface{}) {
	var entries []interface{}
	fbdEntryData, _ := fdbEntry.(nm.FdbEntryStruct)
	entries = Vxlan.translateDeletedFdb(fbdEntryData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Pod.translateDeletedFdb(fbdEntryData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// handleL2NexthopAdded  handles the added l2 nexthop
func handleL2NexthopAdded(l2NextHop interface{}) {
	var entries []interface{}
	l2NextHopData, _ := l2NextHop.(nm.L2NexthopStruct)

	entries = Vxlan.translateAddedL2Nexthop(l2NextHopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Pod.translateAddedL2Nexthop(l2NextHopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// handleL2NexthopUpdated  handles the updated l2 nexthop
func handleL2NexthopUpdated(l2NextHop interface{}) {
	var entries []interface{}
	l2NextHopData, _ := l2NextHop.(nm.L2NexthopStruct)
	entries = Vxlan.translateDeletedL2Nexthop(l2NextHopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Pod.translateDeletedL2Nexthop(l2NextHopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Vxlan.translateDeletedL2Nexthop(l2NextHopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Pod.translateDeletedL2Nexthop(l2NextHopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("iintel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// handleL2NexthopDeleted  handles the deleted l2 nexthop
func handleL2NexthopDeleted(l2NextHop interface{}) {
	var entries []interface{}
	l2NextHopData, _ := l2NextHop.(nm.L2NexthopStruct)
	entries = Vxlan.translateDeletedL2Nexthop(l2NextHopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			err := p4client.DelEntry(e)
			if err != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, err)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	entries = Pod.translateDeletedL2Nexthop(l2NextHopData)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// HandleEvent  handles the infradb events
func (h *ModuleipuHandler) HandleEvent(eventType string, objectData *eventbus.ObjectData) {
	switch eventType {
	case "vrf":
		log.Printf("intel-e2000: recevied %s %s\n", eventType, objectData.Name)
		handlevrf(objectData)
	case "logical-bridge":
		log.Printf("inyel-e2000: recevied %s %s\n", eventType, objectData.Name)
		handlelb(objectData)
	case "bridge-port":
		log.Printf("intel-e2000: recevied %s %s\n", eventType, objectData.Name)
		handlebp(objectData)
	case "svi":
		log.Printf("intel-e2000: recevied %s %s\n", eventType, objectData.Name)
		handlesvi(objectData)
	default:

		log.Println("intel-e2000: error: Unknown event type: ", eventType)
	}
}

// handlevrf  handles the vrf events
func handlevrf(objectData *eventbus.ObjectData) {
	var comp common.Component
	vrf, err := infradb.GetVrf(objectData.Name)
	if err != nil {
		log.Printf("intel-e2000: GetVRF error: %s %s\n", err, objectData.Name)
		return
	}

	if objectData.ResourceVersion != vrf.ResourceVersion {
		log.Printf("intel-e2000: Mismatch in resoruce version %+v\n and vrf resource version %+v\n", objectData.ResourceVersion, vrf.ResourceVersion)
		comp.Name = intele2000Str
		comp.CompStatus = common.ComponentStatusError
		if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err = infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating vrf status: %s\n", err)
		}
		return
	}

	if len(vrf.Status.Components) != 0 {
		for i := 0; i < len(vrf.Status.Components); i++ {
			if vrf.Status.Components[i].Name == intele2000Str {
				comp = vrf.Status.Components[i]
			}
		}
	}
	if vrf.Status.VrfOperStatus != infradb.VrfOperStatusToBeDeleted {
		status := offloadVrf(vrf)
		if status {
			comp.CompStatus = common.ComponentStatusSuccess

			comp.Name = intele2000Str
			comp.Timer = 0
		} else {
			if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2 * time.Second
			}

			comp.Name = intele2000Str
			comp.CompStatus = common.ComponentStatusError
		}
		log.Printf("intel-e2000: %+v\n", comp)
		err = infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, vrf.Metadata, comp)
		if err != nil {
			log.Printf("error in updating vrf status: %s\n", err)
		}
	} else {
		status := tearDownVrf(vrf)
		if status {
			comp.CompStatus = common.ComponentStatusSuccess

			comp.Name = intele2000Str
			comp.Timer = 0
		} else {
			comp.CompStatus = common.ComponentStatusError
			comp.Name = intele2000Str
			if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
				comp.Timer = 2
			} else {
				comp.Timer *= 2
			}
		}

		log.Printf("intel-e2000: %+v\n", comp)
		err = infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating vrf status: %s\n", err)
		}
	}
}

// handlelb  handles the lb events
func handlelb(objectData *eventbus.ObjectData) {
	var comp common.Component
	lb, err := infradb.GetLB(objectData.Name)
	if err != nil {
		log.Printf("intel-e2000: GetLB error: %s %s\n", err, objectData.Name)
		return
	}

	if len(lb.Status.Components) != 0 {
		for i := 0; i < len(lb.Status.Components); i++ {
			if lb.Status.Components[i].Name == intele2000Str {
				comp = lb.Status.Components[i]
			}
		}
	}
	if lb.Status.LBOperStatus != infradb.LogicalBridgeOperStatusToBeDeleted {
		status := setUpLb(lb)
		comp.Name = intele2000Str
		if status {
			comp.Details = ""
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			if comp.Timer == 0 {
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
			comp.CompStatus = common.ComponentStatusError
		}

		log.Printf("intel-e2000: %+v \n", comp)
		err = infradb.UpdateLBStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating lb status: %s\n", err)
		}
	} else {
		status := tearDownLb(lb)
		comp.Name = intele2000Str
		if status {
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			comp.CompStatus = common.ComponentStatusError
			if comp.Timer == 0 {
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
		}

		log.Printf("intel-e2000: %+v\n", comp)
		err = infradb.UpdateLBStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating lb status: %s\n", err)
		}
	}
}

// handlebp  handles the bp events
func handlebp(objectData *eventbus.ObjectData) {
	var comp common.Component
	bp, err := infradb.GetBP(objectData.Name)
	if err != nil {
		log.Printf("intel-e2000: GetBP error: %s\n", err)
		return
	}

	if len(bp.Status.Components) != 0 {
		for i := 0; i < len(bp.Status.Components); i++ {
			if bp.Status.Components[i].Name == intele2000Str {
				comp = bp.Status.Components[i]
			}
		}
	}
	if bp.Status.BPOperStatus != infradb.BridgePortOperStatusToBeDeleted {
		status := setUpBp(bp)
		comp.Name = intele2000Str
		if status {
			comp.Details = ""
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			if comp.Timer == 0 {
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
			comp.CompStatus = common.ComponentStatusError
		}

		log.Printf("intel-e2000: %+v \n", comp)
		err = infradb.UpdateBPStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating bp status: %s\n", err)
		}
	} else {
		status := tearDownBp(bp)
		comp.Name = intele2000Str
		if status {
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			if comp.Timer == 0 {
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
			comp.CompStatus = common.ComponentStatusError
		}

		log.Printf("intel-e2000: %+v \n", comp)
		err = infradb.UpdateBPStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating bp status: %s\n", err)
		}
	}
}

// handlesvi  handles the svi events
func handlesvi(objectData *eventbus.ObjectData) {
	var comp common.Component
	svi, err := infradb.GetSvi(objectData.Name)
	if err != nil {
		log.Printf("intel-e2000: GetSvi error: %s %s\n", err, objectData.Name)
		return
	}

	if objectData.ResourceVersion != svi.ResourceVersion {
		log.Printf("intel-e2000:: Mismatch in resoruce version %+v\n and svi resource version %+v\n", objectData.ResourceVersion, svi.ResourceVersion)
		comp.Name = intele2000Str
		comp.CompStatus = common.ComponentStatusError
		if comp.Timer == 0 {
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err = infradb.UpdateSviStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating svi status: %s\n", err)
		}
		return
	}
	if len(svi.Status.Components) != 0 {
		for i := 0; i < len(svi.Status.Components); i++ {
			if svi.Status.Components[i].Name == intele2000Str {
				comp = svi.Status.Components[i]
			}
		}
	}
	if svi.Status.SviOperStatus != infradb.SviOperStatusToBeDeleted {
		status := setUpSvi(svi)
		comp.Name = intele2000Str
		if status {
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			if comp.Timer == 0 {
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
			comp.CompStatus = common.ComponentStatusError
		}

		log.Printf("intel-e2000:: %+v \n", comp)
		err = infradb.UpdateSviStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating svi status: %s\n", err)
		}
	} else {
		status := tearDownSvi(svi)
		comp.Name = intele2000Str
		if status {
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			comp.CompStatus = common.ComponentStatusError
			if comp.Timer == 0 {
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
		}
		log.Printf("intel-e2000: %+v \n", comp)
		err = infradb.UpdateSviStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating svi status: %s\n", err)
		}
	}
}

// offloadVrf  offload the vrf events
func offloadVrf(vrf *infradb.Vrf) bool {
	if path.Base(vrf.Name) == grdStr {
		return true
	}

	entries := Vxlan.translateAddedVrf(vrf)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("ntel-e2000: Entry is not of type p4client.TableEntry:-", e)
			return false
		}
	}
	return true
}

// setUpLb  set up the logical bridge
func setUpLb(lb *infradb.LogicalBridge) bool {
	entries := Vxlan.translateAddedLb(lb)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry:-", e)
			return false
		}
	}
	return true
}

// setUpBp  set up the bridge port
func setUpBp(bp *infradb.BridgePort) bool {
	// var entries []interface{}
	entries, err := Pod.translateAddedBp(bp)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry:-", e)
			return false
		}
	}
	return true
}

// setUpSvi  set up the svi
func setUpSvi(svi *infradb.Svi) bool {
	// var entries []interface{}
	entries, err := Pod.translateAddedSvi(svi)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry:-", e)
			return false
		}
	}
	return true
}

// tearDownVrf  tear down the vrf
func tearDownVrf(vrf *infradb.Vrf) bool {
	if path.Base(vrf.Name) == grdStr {
		return true
	}
	// var entries []interface{}
	entries := Vxlan.translateDeletedVrf(vrf)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
			return false
		}
	}
	return true
}

// tearDownLb  tear down the logical bridge
func tearDownLb(lb *infradb.LogicalBridge) bool {
	// var entries []interface{}
	entries := Vxlan.translateDeletedLb(lb)
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
			return false
		}
	}
	return true
}

// tearDownBp  tear down the bridge port
func tearDownBp(bp *infradb.BridgePort) bool {
	// var entries []interface{}
	entries, err := Pod.translateDeletedBp(bp)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
			return false
		}
	}
	return true
}

// tearDownSvi  tear down the svi
func tearDownSvi(svi *infradb.Svi) bool {
	// var entries []interface{}
	entries, err := Pod.translateDeletedSvi(svi)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
			return false
		}
	}
	return true
}

// Initialize function handles init functionality
func Initialize() {
	// Netlink Listener
	startSubscriber(nm.EventBus, "route_added")

	startSubscriber(nm.EventBus, "route_updated")
	startSubscriber(nm.EventBus, "route_deleted")
	startSubscriber(nm.EventBus, "nexthop_added")
	startSubscriber(nm.EventBus, "nexthop_updated")
	startSubscriber(nm.EventBus, "nexthop_deleted")
	startSubscriber(nm.EventBus, "fdb_entry_added")
	startSubscriber(nm.EventBus, "fdb_entry_updated")
	startSubscriber(nm.EventBus, "fdb_entry_deleted")
	startSubscriber(nm.EventBus, "l2_nexthop_added")
	startSubscriber(nm.EventBus, "l2_nexthop_updated")
	startSubscriber(nm.EventBus, "l2_nexthop_deleted")

	// InfraDB Listener

	eb := eventbus.EBus
	for _, subscriberConfig := range config.GlobalConfig.Subscribers {
		if subscriberConfig.Name == intele2000Str {
			for _, eventType := range subscriberConfig.Events {
				eb.StartSubscriber(subscriberConfig.Name, eventType, subscriberConfig.Priority, &ModuleipuHandler{})
			}
		}
	}
	// Setup p4runtime connection
	Conn, err := grpc.Dial(defaultAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("intel-e2000: Cannot connect to server: %v\n", err)
	}

	err1 := p4client.NewP4RuntimeClient(config.GlobalConfig.P4.Config.BinFile, config.GlobalConfig.P4.Config.P4infoFile, Conn)
	if err1 != nil {
		log.Fatalf("intel-e2000: Failed to create P4Runtime client: %v\n", err1)
	}
	// add static rules into the pipeline of representators read from config
	representors := make(map[string][2]string)
	for k, v := range config.GlobalConfig.P4.Representors {
		vsi, mac, err := idsOf(v.(string))
		if err != nil {
			log.Println("intel-e2000: Error:", err)
			return
		}
		representors[k] = [2]string{vsi, mac}
	}
	log.Printf("intel-e2000: REPRESENTORS %+v\n", representors)

	L3 = L3.L3DecoderInit(representors)
	Pod = Pod.PodDecoderInit(representors)
	// decoders = []interface{}{L3, Vxlan, Pod}
	Vxlan = Vxlan.VxlanDecoderInit(representors)
	L3entries := L3.StaticAdditions()
	for _, entry := range L3entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	Podentries := Pod.StaticAdditions()
	for _, entry := range Podentries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.AddEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error adding entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}

// DeInitialize function handles stops functionality
func DeInitialize() {
	L3entries := L3.StaticDeletions()
	for _, entry := range L3entries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
	Podentries := Pod.StaticDeletions()
	for _, entry := range Podentries {
		if e, ok := entry.(p4client.TableEntry); ok {
			er := p4client.DelEntry(e)
			if er != nil {
				log.Printf("intel-e2000: error deleting entry for %v error %v\n", e.Tablename, er)
			}
		} else {
			log.Println("intel-e2000: Entry is not of type p4client.TableEntry")
		}
	}
}
