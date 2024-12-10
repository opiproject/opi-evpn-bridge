// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package linuxgeneralmodule is the main package of the application
package linuxgeneralmodule

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"os/exec"
	"reflect"
	"strconv"
	"time"

	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"path"
)

// ModulelgmHandler enmpty interface
type ModulelgmHandler struct{}

// lgmComp string constant
const lgmComp string = "lgm"

// brStr string constant
const brStr string = "br-"

// vxlanStr string constant
const vxlanStr string = "vxlan-"

// routingTableMax max value of routing table
const routingTableMax = 4000

// routingTableMin min value of routing table
const routingTableMin = 1000

// run runs the commands
func run(cmd []string, flag bool) (string, int) {
	var out []byte
	var err error
	out, err = exec.Command(cmd[0], cmd[1:]...).CombinedOutput() //nolint:gosec
	if err != nil {
		if flag {
			panic(fmt.Sprintf("LGM: Command %s': exit code %s;", out, err.Error()))
		}
		log.Printf("LGM: Command %s': exit code %s;\n", out, err)
		return "Error", -1
	}
	output := string(out)
	return output, 0
}

// HandleEvent handles the events with event data
func (h *ModulelgmHandler) HandleEvent(eventType string, objectData *eventbus.ObjectData) {
	switch eventType {
	case "vrf":
		log.Printf("LGM recevied %s %s\n", eventType, objectData.Name)
		handlevrf(objectData)
	case "svi":
		log.Printf("LGM recevied %s %s\n", eventType, objectData.Name)
		handlesvi(objectData)
	case "logical-bridge":
		log.Printf("LGM recevied %s %s\n", eventType, objectData.Name)
		handleLB(objectData)
	default:
		log.Printf("LGM: error: Unknown event type %s", eventType)
	}
}

// handleLB handles the logical Bridge
//
//gocognit:ignore
func handleLB(objectData *eventbus.ObjectData) {
	var comp common.Component
	lb, err := infradb.GetLB(objectData.Name)
	if err != nil {
		log.Printf("LGM: GetLB error: %s %s\n", err, objectData.Name)
		comp.Name = lgmComp
		comp.CompStatus = common.ComponentStatusError
		comp.Details = fmt.Sprintf("LGM: GetLB error: %s %s\n", err, objectData.Name)
		if comp.Timer == 0 {
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateLBStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating lb status: %s\n", err)
		}
		return
	}
	if objectData.ResourceVersion != lb.ResourceVersion {
		log.Printf("LGM: Mismatch in resoruce version %+v\n and lb resource version %+v\n", objectData.ResourceVersion, lb.ResourceVersion)
		comp.Name = lgmComp
		comp.CompStatus = common.ComponentStatusError
		comp.Details = fmt.Sprintf("LGM: Mismatch in resoruce version %+v\n and lb resource version %+v\n", objectData.ResourceVersion, lb.ResourceVersion)
		if comp.Timer == 0 {
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateLBStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating lb status: %s\n", err)
		}
		return
	}
	if len(lb.Status.Components) != 0 {
		for i := 0; i < len(lb.Status.Components); i++ {
			if lb.Status.Components[i].Name == lgmComp {
				comp = lb.Status.Components[i]
			}
		}
	}
	if lb.Status.LBOperStatus != infradb.LogicalBridgeOperStatusToBeDeleted {
		details, status := setUpBridge(lb)
		comp.Name = lgmComp
		comp.Details = details
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
		log.Printf("LGM: %+v \n", comp)
		err := infradb.UpdateLBStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating lb status: %s\n", err)
		}
	} else {
		details, status := tearDownBridge(lb)
		comp.Name = lgmComp
		comp.Details = details
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
		log.Printf("LGM: %+v\n", comp)
		err := infradb.UpdateLBStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating lb status: %s\n", err)
		}
	}
}

// handlesvi handles the svi functionality
//
//gocognit:ignore
func handlesvi(objectData *eventbus.ObjectData) {
	var comp common.Component
	svi, err := infradb.GetSvi(objectData.Name)
	if err != nil {
		log.Printf("LGM: GetSvi error: %s %s\n", err, objectData.Name)
		comp.Name = lgmComp
		comp.CompStatus = common.ComponentStatusError
		if comp.Timer == 0 {
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateSviStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating svi status: %s\n", err)
		}
		return
	}
	if objectData.ResourceVersion != svi.ResourceVersion {
		log.Printf("LGM: Mismatch in resoruce version %+v\n and svi resource version %+v\n", objectData.ResourceVersion, svi.ResourceVersion)
		comp.Name = lgmComp
		comp.CompStatus = common.ComponentStatusError
		comp.Details = fmt.Sprintf("LGM: Mismatch in resoruce version %+v\n and svi resource version %+v\n", objectData.ResourceVersion, svi.ResourceVersion)
		if comp.Timer == 0 {
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateSviStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating svi status: %s\n", err)
		}
		return
	}
	if len(svi.Status.Components) != 0 {
		for i := 0; i < len(svi.Status.Components); i++ {
			if svi.Status.Components[i].Name == lgmComp {
				comp = svi.Status.Components[i]
			}
		}
	}
	if svi.Status.SviOperStatus != infradb.SviOperStatusToBeDeleted {
		details, status := setUpSvi(svi)
		comp.Name = lgmComp
		comp.Details = details
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
		log.Printf("LGM: %+v \n", comp)
		err := infradb.UpdateSviStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating svi status: %s\n", err)
		}
	} else {
		details, status := tearDownSvi(svi)
		comp.Name = lgmComp
		comp.Details = details
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
		log.Printf("LGM: %+v \n", comp)
		err := infradb.UpdateSviStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating svi status: %s\n", err)
		}
	}
}

// handlevrf handles the vrf functionality
//
//gocognit:ignore
func handlevrf(objectData *eventbus.ObjectData) {
	var comp common.Component
	vrf, err := infradb.GetVrf(objectData.Name)
	if err != nil {
		log.Printf("LGM: GetVRF error: %s %s\n", err, objectData.Name)
		comp.Name = lgmComp
		comp.CompStatus = common.ComponentStatusError
		comp.Details = fmt.Sprintf("LGM: GetVRF error: %s %s\n", err, objectData.Name)
		if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating vrf status: %s\n", err)
		}
		return
	}
	if objectData.ResourceVersion != vrf.ResourceVersion {
		log.Printf("LGM: Mismatch in resoruce version %+v\n and vrf resource version %+v\n", objectData.ResourceVersion, vrf.ResourceVersion)
		comp.Name = lgmComp
		comp.CompStatus = common.ComponentStatusError
		comp.Details = fmt.Sprintf("LGM: Mismatch in resoruce version %+v\n and vrf resource version %+v\n", objectData.ResourceVersion, vrf.ResourceVersion)
		if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating vrf status: %s\n", err)
		}
		return
	}
	if len(vrf.Status.Components) != 0 {
		for i := 0; i < len(vrf.Status.Components); i++ {
			if vrf.Status.Components[i].Name == lgmComp {
				comp = vrf.Status.Components[i]
			}
		}
	}
	if vrf.Status.VrfOperStatus != infradb.VrfOperStatusToBeDeleted {
		details, status := setUpVrf(vrf)
		comp.Name = lgmComp
		comp.Details = details
		if status {
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
			comp.CompStatus = common.ComponentStatusError
		}
		log.Printf("LGM: %+v \n", comp)
		err := infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, vrf.Metadata, comp)
		if err != nil {
			log.Printf("error in updating vrf status: %s\n", err)
		}
	} else {
		details, status := tearDownVrf(vrf)
		comp.Name = lgmComp
		comp.Details = details
		if status {
			comp.CompStatus = common.ComponentStatusSuccess
			comp.Timer = 0
		} else {
			comp.CompStatus = common.ComponentStatusError
			if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
		}
		log.Printf("LGM: %+v\n", comp)
		err := infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating vrf status: %s\n", err)
		}
	}
}

// ipMtu variable int
var ipMtu int

// brTenant variable string
var brTenant string

// ctx variable context
var ctx context.Context

// nlink variable wrapper
var nlink utils.Netlink

// RouteTableGen table id generate variable
var RouteTableGen utils.IDPool

// Initialize initializes the config, logger and subscribers
func Initialize() {
	eb := eventbus.EBus
	var ok bool
	for _, subscriberConfig := range config.GlobalConfig.Subscribers {
		if subscriberConfig.Name == lgmComp {
			for _, eventType := range subscriberConfig.Events {
				eb.StartSubscriber(subscriberConfig.Name, eventType, subscriberConfig.Priority, &ModulelgmHandler{})
			}
		}
	}
	brTenant = "br-tenant"
	ipMtu = config.GlobalConfig.LinuxFrr.IPMtu
	ctx = context.Background()
	if RouteTableGen, ok = utils.IDPoolInit("RTtable", routingTableMin, routingTableMax); !ok {
		log.Printf("LGM: Failed in the assigning id \n")
		return
	}
	nlink = utils.NewNetlinkWrapperWithArgs(false)
	// Set up the static configuration parts
	_, err := nlink.LinkByName(ctx, brTenant)
	if err != nil {
		setUpTenantBridge()
	}
}

// DeInitialize function handles stops functionality
func DeInitialize() {
	eb := eventbus.EBus
	err := TearDownTenantBridge()
	if err != nil {
		log.Printf("LGM: Failed to tear down br-tenant: %v\n", err)
	}
	eb.UnsubscribeModule("lgm")
}
func setUpTenantBridge() {
	brTenantMtu := ipMtu + 20
	vlanfiltering := true
	bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: brTenant},
		VlanDefaultPVID: new(uint16),
		VlanFiltering:   &vlanfiltering,
	}

	if err := nlink.LinkAdd(ctx, bridge); err != nil {
		log.Fatalf("LGM: Failed to create br-tenant: %v\n", err)
	}

	if err := nlink.LinkSetMTU(ctx, bridge, brTenantMtu); err != nil {
		log.Fatalf("LGM : Unable to set MTU %v to br-tenant: %v\n", brTenantMtu, err)
	}

	if err := nlink.LinkSetUp(ctx, bridge); err != nil {
		log.Fatalf("LGM: Failed to set up br-tenant: %v\n", err)
	}
}

// routingtableBusy checks if the route is in filterred list
func routingtableBusy(table uint32) (bool, error) {
	routeList, err := nlink.RouteListFiltered(ctx, netlink.FAMILY_V4, &netlink.Route{Table: int(table)}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return false, err
	}
	return len(routeList) > 0, nil
}

// setUpBridge sets up the bridge
func setUpBridge(lb *infradb.LogicalBridge) (string, bool) {
	link := fmt.Sprintf("vxlan-%+v", lb.Spec.VlanID)
	if !reflect.ValueOf(lb.Spec.Vni).IsZero() {
		brIntf, err := nlink.LinkByName(ctx, brTenant)
		if err != nil {
			log.Printf("LGM: Failed to get link information for %s: %v\n", brTenant, err)
			return fmt.Sprintf("LGM: Failed to get link information for %s: %v\n", brTenant, err), false
		}
		vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: link, MTU: ipMtu}, VxlanId: int(*lb.Spec.Vni), Port: 4789, Learning: false, SrcAddr: lb.Spec.VtepIP.IP}
		if err := nlink.LinkAdd(ctx, vxlan); err != nil {
			log.Printf("LGM: Failed to create Vxlan linki %s: %v\n", link, err)
			return fmt.Sprintf("LGM: Failed to create Vxlan linki %s: %v\n", link, err), false
		}
		// Example: ip link set vxlan-<lb-vlan-id> master br-tenant addrgenmode none
		if err = nlink.LinkSetMaster(ctx, vxlan, brIntf); err != nil {
			log.Printf("LGM: Failed to add Vxlan %s to bridge %s: %v\n", link, brTenant, err)
			return fmt.Sprintf("LGM: Failed to add Vxlan %s to bridge %s: %v\n", link, brTenant, err), false
		}
		// Example: ip link set vxlan-<lb-vlan-id> up
		if err = nlink.LinkSetUp(ctx, vxlan); err != nil {
			log.Printf("LGM: Failed to up Vxlan link %s: %v\n", link, err)
			return fmt.Sprintf("LGM: Failed to up Vxlan link %s: %v\n", link, err), false
		}
		// Example: bridge vlan add dev vxlan-<lb-vlan-id> vid <lb-vlan-id> pvid untagged
		if err = nlink.BridgeVlanAdd(ctx, vxlan, uint16(lb.Spec.VlanID), true, true, false, false); err != nil {
			log.Printf("LGM: Failed to add vlan to bridge %s: %v\n", brTenant, err)
			return fmt.Sprintf("LGM: Failed to add vlan to bridge %s: %v\n", brTenant, err), false
		}
		if err = nlink.LinkSetBrNeighSuppress(ctx, vxlan, true); err != nil {
			log.Printf("LGM: Failed to add bridge %v neigh_suppress: %s\n", vxlan, err)
			return fmt.Sprintf("LGM: Failed to add bridge %v neigh_suppress: %s\n", vxlan, err), false
		}

		return "", true
	}
	return "", true
}

// setUpVrf sets up the vrf
//
//nolint:funlen,gocognit
func setUpVrf(vrf *infradb.Vrf) (string, bool) {
	IPMtu := fmt.Sprintf("%+v", ipMtu)
	if path.Base(vrf.Name) == "GRD" {
		vrf.Metadata.RoutingTable = make([]*uint32, 2)
		vrf.Metadata.RoutingTable[0] = new(uint32)
		vrf.Metadata.RoutingTable[1] = new(uint32)
		*vrf.Metadata.RoutingTable[0] = 254
		*vrf.Metadata.RoutingTable[1] = 255
		return "", true
	}
	vrf.Metadata.RoutingTable = make([]*uint32, 1)
	vrf.Metadata.RoutingTable[0] = new(uint32)
	var routingtable uint32
	name := vrf.Name
	routingtable, _ = RouteTableGen.GetID(name, 0)
	log.Printf("LGM assigned id %+v for vrf name %s\n", routingtable, vrf.Name)
	isbusy, err := routingtableBusy(routingtable)
	if err != nil {
		log.Printf("LGM : Error occurred when checking if routing table %d is busy: %+v\n", routingtable, err)
		return "", false
	}
	if !isbusy {
		log.Printf("LGM: Routing Table %d is not busy\n", routingtable)
	}
	var vtip string
	if !reflect.ValueOf(vrf.Spec.VtepIP).IsZero() {
		vtip = fmt.Sprintf("%+v", vrf.Spec.VtepIP.IP)
		// Verify that the specified VTEP IP exists as local IP
		err := nlink.RouteListIPTable(ctx, vtip)
		// Not found similar API in viswananda library so retain the linux commands as it is .. not able to get the route list exact vtip table local
		if !err {
			log.Printf(" LGM: VTEP IP not found: %+v\n", vrf.Spec.VtepIP)
			return fmt.Sprintf(" LGM: VTEP IP not found: %+v\n", vrf.Spec.VtepIP), false
		}
	}
	log.Printf("setUpVrf: %s %d\n", vtip, routingtable)
	// Create the vrf interface for the specified routing table and add loopback address

	linkAdderr := nlink.LinkAdd(ctx, &netlink.Vrf{
		LinkAttrs: netlink.LinkAttrs{Name: path.Base(vrf.Name)},
		Table:     routingtable,
	})
	if linkAdderr != nil {
		log.Printf("LGM: Error in Adding vrf link table %d\n", routingtable)
		return fmt.Sprintf("LGM: Error in Adding vrf link table %d\n", routingtable), false
	}

	log.Printf("LGM: vrf link %s Added with table id %d\n", vrf.Name, routingtable)

	link, linkErr := nlink.LinkByName(ctx, path.Base(vrf.Name))
	if linkErr != nil {
		log.Printf("LGM : Link %s not found\n", vrf.Name)
		return fmt.Sprintf("LGM : Link %s not found\n", vrf.Name), false
	}

	linkmtuErr := nlink.LinkSetMTU(ctx, link, ipMtu)
	if linkmtuErr != nil {
		log.Printf("LGM : Unable to set MTU to link %s \n", vrf.Name)
		return fmt.Sprintf("LGM : Unable to set MTU to link %s \n", vrf.Name), false
	}

	linksetupErr := nlink.LinkSetUp(ctx, link)
	if linksetupErr != nil {
		log.Printf("LGM : Unable to set link %s UP \n", vrf.Name)
		return fmt.Sprintf("LGM : Unable to set link %s UP \n", vrf.Name), false
	}
	Lbip := fmt.Sprintf("%+v", vrf.Spec.LoopbackIP.IP)

	var address = vrf.Spec.LoopbackIP
	var Addrs = &netlink.Addr{
		IPNet: address,
	}
	addrErr := nlink.AddrAdd(ctx, link, Addrs)
	if addrErr != nil {
		log.Printf("LGM: Unable to set the loopback ip to vrf link %s \n", vrf.Name)
		return fmt.Sprintf("LGM: Unable to set the loopback ip to vrf link %s \n", vrf.Name), false
	}

	log.Printf("LGM: Added Address %s dev %s\n", Lbip, vrf.Name)

	Src1 := net.IPv4(0, 0, 0, 0)
	route := netlink.Route{
		Table:    int(routingtable),
		Type:     unix.RTN_THROW,
		Protocol: 255,
		Priority: 9999,
		Src:      Src1,
	}
	routeaddErr := nlink.RouteAdd(ctx, &route)
	if routeaddErr != nil {
		log.Printf("LGM : Failed in adding Route throw default %+v\n", routeaddErr)
		return fmt.Sprintf("LGM : Failed in adding Route throw default %+v\n", routeaddErr), false
	}

	log.Printf("LGM : Added route throw default table %d proto opi_evpn_br metric 9999\n", routingtable)
	// Disable reverse-path filtering to accept ingress traffic punted by the pipeline
	// disable_rp_filter("rep-"+vrf.Name)
	// Configuration specific for VRFs associated with L3 EVPN
	if !reflect.ValueOf(vrf.Spec.Vni).IsZero() {
		// Create bridge for external VXLAN under vrf
		// Linux apparently creates a deterministic MAC address for a bridge type link with a given
		// name. We need to assign a true random MAC address to avoid collisions when pairing two
		// servers.

		brErr := nlink.LinkAdd(ctx, &netlink.Bridge{
			LinkAttrs: netlink.LinkAttrs{Name: brStr + path.Base(vrf.Name)},
		})
		if brErr != nil {
			log.Printf("LGM : Error in added bridge port\n")
			return fmt.Sprintf("LGM : Error in added bridge port %v", brErr), false
		}
		log.Printf("LGM : Added link br-%s type bridge\n", vrf.Name)

		rmac := fmt.Sprintf("%+v", GenerateMac()) // str(macaddress.MAC(b'\x00'+random.randbytes(5))).replace("-", ":")
		hw, _ := net.ParseMAC(rmac)

		linkBr, brErr := nlink.LinkByName(ctx, brStr+path.Base(vrf.Name))
		if brErr != nil {
			log.Printf("LGM : Error in getting the br-%s\n", vrf.Name)
			return fmt.Sprintf("LGM : Error in getting the br-%s\n", vrf.Name), false
		}
		hwErr := nlink.LinkSetHardwareAddr(ctx, linkBr, hw)
		if hwErr != nil {
			log.Printf("LGM: Failed in the setting Hardware Address\n")
			return fmt.Sprintf("LGM: Failed in the setting Hardware Address: %v\n", hwErr), false
		}

		linkmtuErr := nlink.LinkSetMTU(ctx, linkBr, ipMtu)
		if linkmtuErr != nil {
			log.Printf("LGM : Unable to set MTU to link br-%s \n", vrf.Name)
			return fmt.Sprintf("LGM : Unable to set MTU to link br-%s \n", vrf.Name), false
		}

		linkMaster, errMaster := nlink.LinkByName(ctx, path.Base(vrf.Name))
		if errMaster != nil {
			log.Printf("LGM : Error in getting the %s\n", vrf.Name)
			return fmt.Sprintf("LGM : Error in getting the %s\n", vrf.Name), false
		}

		err := nlink.LinkSetMaster(ctx, linkBr, linkMaster)
		if err != nil {
			log.Printf("LGM : Unable to set the master to br-%s link", vrf.Name)
			return fmt.Sprintf("LGM : Unable to set the master to br-%s link", vrf.Name), false
		}

		linksetupErr = nlink.LinkSetUp(ctx, linkBr)
		if linksetupErr != nil {
			log.Printf("LGM : Unable to set link %s UP \n", vrf.Name)
			return fmt.Sprintf("LGM : Unable to set link %s UP \n", vrf.Name), false
		}
		log.Printf("LGM: link set  br-%s master  %s up mtu \n", vrf.Name, IPMtu)

		// Create the VXLAN link in the external bridge

		SrcVtep := vrf.Spec.VtepIP.IP
		vxlanErr := nlink.LinkAdd(ctx, &netlink.Vxlan{
			LinkAttrs: netlink.LinkAttrs{Name: vxlanStr + path.Base(vrf.Name), MTU: ipMtu}, VxlanId: int(*vrf.Spec.Vni), SrcAddr: SrcVtep, Learning: false, Proxy: true, Port: 4789})
		if vxlanErr != nil {
			log.Printf("LGM : Error in added vxlan port\n")
			return fmt.Sprintf("LGM : Error in added vxlan port %v\n", vxlanErr), false
		}

		log.Printf("LGM : link added vxlan-%s type vxlan id %d local %s dstport 4789 nolearning proxy\n", vrf.Name, *vrf.Spec.Vni, vtip)

		linkVxlan, vxlanErr := nlink.LinkByName(ctx, vxlanStr+path.Base(vrf.Name))
		if vxlanErr != nil {
			log.Printf("LGM : Error in getting the %s\n", vxlanStr+vrf.Name)
			return fmt.Sprintf("LGM : Error in getting the %s\n", vxlanStr+vrf.Name), false
		}

		err = nlink.LinkSetMaster(ctx, linkVxlan, linkBr)
		if err != nil {
			log.Printf("LGM : Unable to set the master to vxlan-%s link", vrf.Name)
			return fmt.Sprintf("LGM : Unable to set the master to vxlan-%s link", vrf.Name), false
		}

		log.Printf("LGM: vrf Link vxlan setup master\n")

		linksetupErr = nlink.LinkSetUp(ctx, linkVxlan)
		if linksetupErr != nil {
			log.Printf("LGM : Unable to set link %s UP \n", vrf.Name)
			return fmt.Sprintf("LGM : Unable to set link %s UP \n", vrf.Name), false
		}
	}
	details := fmt.Sprintf("{\"routingtable\":\"%d\"}", routingtable)
	*vrf.Metadata.RoutingTable[0] = routingtable
	return details, true
}

// setUpSvi sets up the svi
func setUpSvi(svi *infradb.Svi) (string, bool) {
	BrObj, err := infradb.GetLB(svi.Spec.LogicalBridge)
	if err != nil {
		log.Printf("LGM: unable to find key %s and error is %v", svi.Spec.LogicalBridge, err)
		return fmt.Sprintf("LGM: unable to find key %s and error is %v", svi.Spec.LogicalBridge, err), false
	}
	linkSvi := fmt.Sprintf("%+v-%+v", path.Base(svi.Spec.Vrf), BrObj.Spec.VlanID)
	brIntf, err := nlink.LinkByName(ctx, brTenant)
	if err != nil {
		log.Printf("LGM : Failed to get link information for %s: %v\n", brTenant, err)
		return fmt.Sprintf("LGM : Failed to get link information for %s: %v\n", brTenant, err), false
	}
	if BrObj.Spec.VlanID > math.MaxUint16 {
		log.Printf("LGM : VlanID %v value passed in Logical Bridge create is greater than 16 bit value\n", BrObj.Spec.VlanID)
		return fmt.Sprintf("LGM : VlanID %v value passed in Logical Bridge create is greater than 16 bit value\n", BrObj.Spec.VlanID), false
	}
	vid := uint16(BrObj.Spec.VlanID)
	if err = nlink.BridgeVlanAdd(ctx, brIntf, vid, false, false, true, false); err != nil {
		log.Printf("LGM : Failed to add VLAN %d to bridge interface %s: %v\n", vid, brTenant, err)
		return fmt.Sprintf("LGM : Failed to add VLAN %d to bridge interface %s: %v\n", vid, brTenant, err), false
	}
	log.Printf("LGM Executed : bridge vlan add dev %s vid %d self\n", brTenant, vid)

	vlanLink := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: linkSvi, ParentIndex: brIntf.Attrs().Index}, VlanId: int(BrObj.Spec.VlanID)}
	if err = nlink.LinkAdd(ctx, vlanLink); err != nil {
		log.Printf("LGM : Failed to add VLAN sub-interface %s: %v\n", linkSvi, err)
		return fmt.Sprintf("LGM : Failed to add VLAN sub-interface %s: %v\n", linkSvi, err), false
	}

	log.Printf("LGM Executed : ip link add link %s name %s type vlan id %d\n", brTenant, linkSvi, vid)
	if err = nlink.LinkSetHardwareAddr(ctx, vlanLink, *svi.Spec.MacAddress); err != nil {
		log.Printf("LGM : Failed to set link %v: %s\n", vlanLink, err)
		return fmt.Sprintf("LGM : Failed to set link %v: %s\n", vlanLink, err), false
	}

	log.Printf("LGM Executed : ip link set %s address %s\n", linkSvi, *svi.Spec.MacAddress)
	vrfIntf, err := nlink.LinkByName(ctx, path.Base(svi.Spec.Vrf))
	if err != nil {
		log.Printf("LGM : Failed to get link information for %s: %v\n", path.Base(svi.Spec.Vrf), err)
		return fmt.Sprintf("LGM : Failed to get link information for %s: %v\n", path.Base(svi.Spec.Vrf), err), false
	}
	if err = nlink.LinkSetMaster(ctx, vlanLink, vrfIntf); err != nil {
		log.Printf("LGM : Failed to set master for %v: %s\n", vlanLink, err)
		return fmt.Sprintf("LGM : Failed to set master for %v: %s\n", vlanLink, err), false
	}
	if err = nlink.LinkSetUp(ctx, vlanLink); err != nil {
		log.Printf("LGM : Failed to set up link for %v: %s\n", vlanLink, err)
		return fmt.Sprintf("LGM : Failed to set up link for %v: %s\n", vlanLink, err), false
	}
	if err = nlink.LinkSetMTU(ctx, vlanLink, ipMtu); err != nil {
		log.Printf("LGM : Failed to set MTU for %v: %s\n", vlanLink, err)
		return fmt.Sprintf("LGM : Failed to set MTU for %v: %s\n", vlanLink, err), false
	}

	log.Printf("LGM Executed :  ip link set %s master %s up mtu %d\n", linkSvi, path.Base(svi.Spec.Vrf), ipMtu)
	// Ignoring the error as CI env doesn't allow to write to the filesystem
	command := fmt.Sprintf("net.ipv4.conf.%s.arp_accept=1", linkSvi)
	CP, err1 := run([]string{"sysctl", "-w", command}, false)
	if err1 != 0 {
		log.Printf("LGM: Error in executing command %s %s\n", "sysctl -w net.ipv4.conf.linkSvi.arp_accept=1", linkSvi)
		log.Printf("%s\n", CP)
		// return  false
	}
	for _, ipIntf := range svi.Spec.GatewayIPs {
		addr := &netlink.Addr{
			IPNet: &net.IPNet{
				IP:   ipIntf.IP,
				Mask: ipIntf.Mask,
			},
		}
		if err := nlink.AddrAdd(ctx, vlanLink, addr); err != nil {
			log.Printf("LGM: Failed to add ip address %v to %v: %v\n", addr, vlanLink, err)
			return fmt.Sprintf("LGM: Failed to add ip address %v to %v: %v\n", addr, vlanLink, err), false
		}

		log.Printf("LGM Executed :  ip address add %s dev %+v\n", addr, vlanLink)
	}
	return "", true
}

// GenerateMac Generates the random mac
func GenerateMac() net.HardwareAddr {
	buf := make([]byte, 5)
	var mac net.HardwareAddr
	_, err := rand.Read(buf) //nolint:gosec
	if err != nil {
		log.Printf("failed to generate random mac %+v\n", err)
	}

	// Set the local bit
	//  buf[0] |= 8

	mac = append(mac, 00, buf[0], buf[1], buf[2], buf[3], buf[4])

	return mac
}

// NetMaskToInt convert network mask to int value
func NetMaskToInt(mask int) (netmaskint [4]int64) {
	var binarystring string

	for ii := 1; ii <= mask; ii++ {
		binarystring += "1"
	}
	for ii := 1; ii <= (32 - mask); ii++ {
		binarystring += "0"
	}
	oct1 := binarystring[0:8]
	oct2 := binarystring[8:16]
	oct3 := binarystring[16:24]
	oct4 := binarystring[24:]

	netmaskint[0], _ = strconv.ParseInt(oct1, 2, 64)
	netmaskint[1], _ = strconv.ParseInt(oct2, 2, 64)
	netmaskint[2], _ = strconv.ParseInt(oct3, 2, 64)
	netmaskint[3], _ = strconv.ParseInt(oct4, 2, 64)

	return netmaskint
}

// tearDownVrf tears down the vrf
func tearDownVrf(vrf *infradb.Vrf) (string, bool) {
	link, err1 := nlink.LinkByName(ctx, path.Base(vrf.Name))
	if err1 != nil {
		log.Printf("LGM : Link %s not found %+v\n", vrf.Name, err1)
		return fmt.Sprintf("LGM : Link %s not found %+v\n", vrf.Name, err1), true
	}

	if path.Base(vrf.Name) == "GRD" {
		return "", true
	}
	routingtable := *vrf.Metadata.RoutingTable[0]
	// Delete the Linux networking artefacts in reverse order
	if !reflect.ValueOf(vrf.Spec.Vni).IsZero() {
		linkVxlan, linkErr := nlink.LinkByName(ctx, vxlanStr+path.Base(vrf.Name))
		if linkErr != nil {
			log.Printf("LGM : Link vxlan-%s not found %+v\n", vrf.Name, linkErr)
			return fmt.Sprintf("LGM : Link vxlan-%s not found %+v\n", vrf.Name, linkErr), false
		}
		delerr := nlink.LinkDel(ctx, linkVxlan)
		if delerr != nil {
			log.Printf("LGM: Error in delete vxlan %+v\n", delerr)
			return fmt.Sprintf("LGM: Error in delete vxlan %+v\n", delerr), false
		}
		log.Printf("LGM : Delete vxlan-%s\n", vrf.Name)

		linkBr, linkbrErr := nlink.LinkByName(ctx, brStr+path.Base(vrf.Name))
		if linkbrErr != nil {
			log.Printf("LGM : Link br-%s not found %+v\n", vrf.Name, linkbrErr)
			return fmt.Sprintf("LGM : Link br-%s not found %+v\n", vrf.Name, linkbrErr), false
		}
		delerr = nlink.LinkDel(ctx, linkBr)
		if delerr != nil {
			log.Printf("LGM: Error in delete br %+v\n", delerr)
			return fmt.Sprintf("LGM: Error in delete br %+v\n", delerr), false
		}
		log.Printf("LGM : Delete br-%s\n", vrf.Name)
	}
	routeTable := fmt.Sprintf("%+v", routingtable)
	flusherr := nlink.RouteFlushTable(ctx, routeTable)
	if flusherr != nil {
		log.Printf("LGM: Error in flush table  %+v\n", routeTable)
		return fmt.Sprintf("LGM: Error in flush table  %+v\n", routeTable), false
	}
	log.Printf("LGM Executed : ip route flush table %s\n", routeTable)
	delerr := nlink.LinkDel(ctx, link)
	if delerr != nil {
		log.Printf("LGM: Error in delete br %+v\n", delerr)
		return fmt.Sprintf("LGM: Error in delete br %+v\n", delerr), false
	}
	log.Printf("LGM :link delete  %s\n", vrf.Name)
	return "", true
}

// tearDownSvi tears down the svi
func tearDownSvi(svi *infradb.Svi) (string, bool) {
	BrObj, err := infradb.GetLB(svi.Spec.LogicalBridge)
	if err != nil {
		log.Printf("LGM: unable to find key %s and error is %v", svi.Spec.LogicalBridge, err)
		return fmt.Sprintf("LGM: unable to find key %s and error is %v", svi.Spec.LogicalBridge, err), false
	}
	brIntf, err := nlink.LinkByName(ctx, brTenant)
	if err != nil {
		log.Printf("LGM : Failed to get link information for %s: %v\n", brTenant, err)
		return fmt.Sprintf("LGM : Failed to get link information for %s: %v\n", brTenant, err), false
	}
	if BrObj.Spec.VlanID > math.MaxUint16 {
		log.Printf("LGM : VlanID %v value passed in Logical Bridge create is greater than 16 bit value\n", BrObj.Spec.VlanID)
		return fmt.Sprintf("LGM : VlanID %v value passed in Logical Bridge create is greater than 16 bit value\n", BrObj.Spec.VlanID), false
	}
	vid := uint16(BrObj.Spec.VlanID)
	if err = nlink.BridgeVlanDel(ctx, brIntf, vid, false, false, true, false); err != nil {
		log.Printf("LGM : Failed to Del VLAN %d to bridge interface %s: %v\n", vid, brTenant, err)
		return fmt.Sprintf("LGM : Failed to Del VLAN %d to bridge interface %s: %v\n", vid, brTenant, err), false
	}
	log.Printf("LGM Executed : bridge vlan del dev %s vid %d self\n", brTenant, vid)
	linkSvi := fmt.Sprintf("%+v-%+v", path.Base(svi.Spec.Vrf), BrObj.Spec.VlanID)
	Intf, err := nlink.LinkByName(ctx, linkSvi)
	if err != nil {
		log.Printf("LGM : Failed to get link %s: %v\n", linkSvi, err)
		return fmt.Sprintf("LGM : Failed to get link %s: %v\n", linkSvi, err), true
	}

	if err = nlink.LinkDel(ctx, Intf); err != nil {
		log.Printf("LGM : Failed to delete link %s: %v\n", linkSvi, err)
		return fmt.Sprintf("LGM : Failed to delete link %s: %v\n", linkSvi, err), false
	}
	log.Printf("LGM: Executed ip link delete %s\n", linkSvi)

	return "", true
}

// tearDownBridge tears down the bridge
func tearDownBridge(lb *infradb.LogicalBridge) (string, bool) {
	link := fmt.Sprintf("vxlan-%+v", lb.Spec.VlanID)
	if !reflect.ValueOf(lb.Spec.Vni).IsZero() {
		Intf, err := nlink.LinkByName(ctx, link)
		if err != nil {
			log.Printf("LGM: Failed to get link %s: %v\n", link, err)
			return fmt.Sprintf("LGM: Failed to get link %s: %v\n", link, err), true
		}
		if err = nlink.LinkDel(ctx, Intf); err != nil {
			log.Printf("LGM : Failed to delete link %s: %v\n", link, err)
			return fmt.Sprintf("LGM: Failed to delete link %s: %v\n", link, err), false
		}
		log.Printf("LGM: Executed ip link delete %s", link)

		return "", true
	}
	return "", true
}

// TearDownTenantBridge tears down the bridge
func TearDownTenantBridge() error {
	Intf, err := nlink.LinkByName(ctx, brTenant)
	if err != nil {
		log.Printf("LGM: Failed to get br-tenant %s: %v\n", Intf, err)
		return err
	}
	if err = nlink.LinkDel(ctx, Intf); err != nil {
		log.Printf("LGM : Failed to delete br-tenant %s: %v\n", Intf, err)
		return err
	}
	log.Printf("LGM: Executed ip link delete %s", brTenant)

	return nil
}
