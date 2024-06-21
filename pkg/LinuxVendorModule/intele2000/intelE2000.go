// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package intele2000 handles intel e2000 vendor specific tasks
package intele2000

import (
	"context"
	"fmt"

	"log"
	"math"
	"net"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	"github.com/vishvananda/netlink"
)

// portMux variable of type string
var portMux string

// vrfMux variable of type string
var vrfMux string

// ModulelvmHandler empty interface
type ModulelvmHandler struct{}

// lvmComp empty interface
const lvmComp string = "lvm"

// run runs the command
func run(cmd []string, flag bool) (string, int) {
	var out []byte
	var err error
	out, err = exec.Command(cmd[0], cmd[1:]...).CombinedOutput() //nolint:gosec
	if err != nil {
		if flag {
			panic(fmt.Sprintf("LVM: Command %s': exit code %s;", out, err.Error()))
		}
		log.Printf("LVM: Command %s': exit code %s;\n", out, err)
		return "Error", -1
	}
	output := string(out)
	return output, 0
}

// HandleEvent handles the events
func (h *ModulelvmHandler) HandleEvent(eventType string, objectData *eventbus.ObjectData) {
	switch eventType {
	case "vrf":
		log.Printf("LVM recevied %s %s\n", eventType, objectData.Name)
		handlevrf(objectData)
	case "bridge-port":
		log.Printf("LVM recevied %s %s\n", eventType, objectData.Name)
		handlebp(objectData)
	default:
		log.Printf("error: Unknown event type %s\n", eventType)
	}
}

// handlebp  handles the bridge port functionality
//
//gocognit:ignore
func handlebp(objectData *eventbus.ObjectData) {
	var comp common.Component
	bp, err := infradb.GetBP(objectData.Name)
	if err != nil {
		log.Printf("LVM : GetBP error: %s\n", err)
		comp.Name = lvmComp
		comp.CompStatus = common.ComponentStatusError
		if comp.Timer == 0 {
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateBPStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating bp status: %s\n", err)
		}
		return
	}
	if objectData.ResourceVersion != bp.ResourceVersion {
		log.Printf("LVM: Mismatch in resoruce version %+v\n and bp resource version %+v\n", objectData.ResourceVersion, bp.ResourceVersion)
		comp.Name = lvmComp
		comp.CompStatus = common.ComponentStatusError
		if comp.Timer == 0 {
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateBPStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating bp status: %s\n", err)
		}
		return
	}
	if len(bp.Status.Components) != 0 {
		for i := 0; i < len(bp.Status.Components); i++ {
			if bp.Status.Components[i].Name == lvmComp {
				comp = bp.Status.Components[i]
			}
		}
	}
	if bp.Status.BPOperStatus != infradb.BridgePortOperStatusToBeDeleted {
		status := setUpBp(bp)
		comp.Name = lvmComp
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
		log.Printf("LVM: %+v \n", comp)
		err := infradb.UpdateBPStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, bp.Metadata, comp)
		if err != nil {
			log.Printf("error updaing bp status %s\n", err)
		}
	} else {
		status := tearDownBp(bp)
		comp.Name = lvmComp
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
		log.Printf("LVM: %+v \n", comp)
		err := infradb.UpdateBPStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error updaing bp status %s\n", err)
		}
	}
}

// MactoVport converts mac address to vport
func MactoVport(mac *net.HardwareAddr) int {
	byte0 := int((*mac)[0])
	byte1 := int((*mac)[1])
	return (byte0 << 8) + byte1
}

// setUpBp sets up a bridge port
func setUpBp(bp *infradb.BridgePort) bool {
	MacAddress := fmt.Sprintf("%+v", *bp.Spec.MacAddress)
	vportID := MactoVport(bp.Spec.MacAddress)
	link := fmt.Sprintf("vport-%+v", vportID)
	vport := fmt.Sprintf("%+v", vportID)
	bp.Metadata.VPort = vport
	muxIntf, err := nlink.LinkByName(ctx, portMux)
	if err != nil {
		log.Printf("Failed to get link information for %s, error is %v\n", portMux, err)
		return false
	}
	vlanLink := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: link, ParentIndex: muxIntf.Attrs().Index}, VlanId: vportID, VlanProtocol: netlink.VLAN_PROTOCOL_8021AD}
	if err = nlink.LinkAdd(ctx, vlanLink); err != nil {
		log.Printf("Failed to add VLAN sub-interface %s: %v\n", link, err)
		return false
	}
	log.Printf("LVM: Executed ip link add link %s name %s type vlan protocol 802.1ad id %s\n", portMux, link, vport)
	brIntf, err := nlink.LinkByName(ctx, brTenant)
	if err != nil {
		log.Printf("Failed to get link information for %s: %v\n", brTenant, err)
		return false
	}
	if err = nlink.LinkSetMaster(ctx, vlanLink, brIntf); err != nil {
		log.Printf("Failed to set master for %s: %v\n", brIntf, err)
		return false
	}
	if err = nlink.LinkSetUp(ctx, vlanLink); err != nil {
		log.Printf("Failed to set up link for %v: %s\n", vlanLink, err)
		return false
	}
	if err = nlink.LinkSetMTU(ctx, vlanLink, ipMtu); err != nil {
		log.Printf("Failed to set MTU for %v: %s\n", vlanLink, err)
		return false
	}
	log.Printf("LVM: Executed ip link set %s master %s up mtu %d \n", link, brTenant, ipMtu)
	for _, vlan := range bp.Spec.LogicalBridges {
		BrObj, err := infradb.GetLB(vlan)
		if err != nil {
			log.Printf("LVM: unable to find key %s and error is %v", vlan, err)
			return false
		}
		if BrObj.Spec.VlanID > math.MaxUint16 {
			log.Printf("LVM : VlanID %v value passed in Logical Bridge create is greater than 16 bit value\n", BrObj.Spec.VlanID)
			return false
		}
		//TODO: Update opi-api to change vlanid to uint16 in LogiclaBridge
		vid := uint16(BrObj.Spec.VlanID)
		if err = nlink.BridgeVlanAdd(ctx, vlanLink, vid, false, false, false, false); err != nil {
			log.Printf("Failed to add VLAN %d to bridge interface %s: %v\n", vportID, link, err)
			return false
		}
		log.Printf("LVM: Executed bridge vlan add dev %s vid %d \n", link, vid)
	}
	if err = nlink.BridgeFdbAdd(ctx, link, MacAddress); err != nil {
		log.Printf("LVM: Error in executing command %s %s with error %s\n", "bridge fdb add", link, err)
		return false
	}
	log.Printf("LVM: Executed bridge fdb add %s dev %s master static extern_learn\n", MacAddress, link)
	return true
}

// tearDownBp tears down the bridge port
func tearDownBp(bp *infradb.BridgePort) bool {
	vportID := MactoVport(bp.Spec.MacAddress)
	link := fmt.Sprintf("vport-%+v", vportID)
	Intf, err := nlink.LinkByName(ctx, link)
	if err != nil {
		log.Printf("Failed to get link %v: %s\n", link, err)
		return true
	}
	if err = nlink.LinkDel(ctx, Intf); err != nil {
		log.Printf("Failed to delete link %v: %s\n", link, err)
		return false
	}
	log.Printf(" LVM: Executed ip link delete %v\n", link)
	return true
}

// handlevrf handles the vrf functionality
//
//gocognit:ignore
func handlevrf(objectData *eventbus.ObjectData) {
	var comp common.Component
	vrf, err := infradb.GetVrf(objectData.Name)
	if err != nil {
		log.Printf("LVM : GetVrf error: %s\n", err)
		comp.Name = lvmComp
		comp.CompStatus = common.ComponentStatusError
		if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error updaing vrf status %s\n", err)
		}
		return
	}
	if objectData.ResourceVersion != vrf.ResourceVersion {
		log.Printf("LVM: Mismatch in resoruce version %+v\n and vrf resource version %+v\n", objectData.ResourceVersion, vrf.ResourceVersion)
		comp.Name = lvmComp
		comp.CompStatus = common.ComponentStatusError
		if comp.Timer == 0 { // wait timer is 2 powerof natural numbers ex : 1,2,3...
			comp.Timer = 2 * time.Second
		} else {
			comp.Timer *= 2
		}
		err := infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error updaing vrf status %s\n", err)
		}
		return
	}
	if len(vrf.Status.Components) != 0 {
		for i := 0; i < len(vrf.Status.Components); i++ {
			if vrf.Status.Components[i].Name == lvmComp {
				comp = vrf.Status.Components[i]
			}
		}
	}
	if vrf.Status.VrfOperStatus != infradb.VrfOperStatusToBeDeleted {
		statusUpdate := setUpVrf(vrf)
		comp.Name = lvmComp
		if statusUpdate {
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
		err := infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error updaing vrf status %s\n", err)
		}
	} else {
		comp.Name = lvmComp
		if tearDownVrf(vrf) {
			comp.CompStatus = common.ComponentStatusSuccess
		} else {
			if comp.Timer == 0 {
				comp.Timer = 2 * time.Second
			} else {
				comp.Timer *= 2
			}
			comp.CompStatus = common.ComponentStatusError
		}
		err := infradb.UpdateVrfStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error updaing vrf status %s\n", err)
		}
	}
}

// disableRpFilter disables the RP filter
func disableRpFilter(iface string) {
	// Work-around for the observation that sometimes the sysctl -w command did not take effect.
	rpFilterDisabled := false
	for i := 0; i < 3; i++ {
		rpDisable := fmt.Sprintf("net.ipv4.conf.%s.rp_filter=0", iface)
		run([]string{"sysctl", "-w", rpDisable}, false)
		time.Sleep(2 * time.Millisecond)
		rpDisable = fmt.Sprintf("net.ipv4.conf.%s.rp_filter", iface)
		CP, err := run([]string{"sysctl", "-n", rpDisable}, false)
		if err == 0 && strings.HasPrefix(CP, "0") {
			rpFilterDisabled = true
			log.Printf("LVM: rpFilterDisabled: %+v\n", rpFilterDisabled)
			break
		}
	}
	if !rpFilterDisabled {
		log.Printf("Failed to disable rp_filtering on interface %s\n", iface)
	}
}

// setUpVrf sets up a vrf
func setUpVrf(vrf *infradb.Vrf) bool {
	log.Printf("LVM configure linux function \n")
	vlanIntf := fmt.Sprintf("rep-%+v", path.Base(vrf.Name))
	if path.Base(vrf.Name) == "GRD" {
		disableRpFilter("rep-" + path.Base(vrf.Name))
		return true
	}
	muxIntf, err := nlink.LinkByName(ctx, vrfMux)
	if err != nil {
		log.Printf("Failed to get link information for %s, error is %v\n", vrfMux, err)
		return false
	}
	vlanLink := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanIntf, ParentIndex: muxIntf.Attrs().Index}, VlanId: int(*vrf.Metadata.RoutingTable[0])}
	if err = nlink.LinkAdd(ctx, vlanLink); err != nil {
		log.Printf("Failed to add VLAN sub-interface %s: %v\n", vlanIntf, err)
		return false
	}
	log.Printf(" LVM: Executed ip link add link %s name rep-%s type vlan id %s\n", vrfMux, path.Base(vrf.Name), strconv.Itoa(int(*vrf.Metadata.RoutingTable[0])))
	vrfIntf, err := nlink.LinkByName(ctx, path.Base(vrf.Name))
	if err != nil {
		log.Printf("Failed to get link information for %s: %v\n", path.Base(vrf.Name), err)
		return false
	}
	if err = nlink.LinkSetMaster(ctx, vlanLink, vrfIntf); err != nil {
		log.Printf("Failed to set master for %v: %s\n", vlanIntf, err)
		return false
	}
	if err = nlink.LinkSetUp(ctx, vlanLink); err != nil {
		log.Printf("Failed to set up link for %v: %s\n", vlanLink, err)
		return false
	}
	if err = nlink.LinkSetMTU(ctx, vlanLink, ipMtu); err != nil {
		log.Printf("Failed to set MTU for %v: %s\n", vlanLink, err)
		return false
	}
	log.Printf(" LVM: Executed ip link set rep-%s master %s up mtu %d\n", path.Base(vrf.Name), path.Base(vrf.Name), ipMtu)
	disableRpFilter("rep-" + path.Base(vrf.Name))
	return true
}

// tearDownVrf tears down a vrf
func tearDownVrf(vrf *infradb.Vrf) bool {
	vlanIntf := fmt.Sprintf("rep-%+v", path.Base(vrf.Name))
	if path.Base(vrf.Name) == "GRD" {
		return true
	}
	Intf, err := nlink.LinkByName(ctx, vlanIntf)
	if err != nil {
		log.Printf("Failed to get link %v: %s\n", vlanIntf, err)
		return false
	}
	if err = nlink.LinkDel(ctx, Intf); err != nil {
		log.Printf("Failed to delete link %v: %s\n", vlanIntf, err)
		return false
	}
	log.Printf(" LVM: Executed ip link delete rep-%s\n", path.Base(vrf.Name))
	return true
}

var ipMtu int
var brTenant string
var ctx context.Context
var nlink utils.Netlink

// Initialize function initialize config
func Initialize() {
	eb := eventbus.EBus
	for _, subscriberConfig := range config.GlobalConfig.Subscribers {
		if subscriberConfig.Name == lvmComp {
			for _, eventType := range subscriberConfig.Events {
				eb.StartSubscriber(subscriberConfig.Name, eventType, subscriberConfig.Priority, &ModulelvmHandler{})
			}
		}
	}
	portMux = config.GlobalConfig.LinuxFrr.PortMux
	vrfMux = config.GlobalConfig.LinuxFrr.VrfMux
	ipMtu = config.GlobalConfig.LinuxFrr.IPMtu
	brTenant = "br-tenant"
	ctx = context.Background()
	nlink = utils.NewNetlinkWrapperWithArgs(config.GlobalConfig.Tracer)
}

// DeInitialize function handles stops functionality
func DeInitialize() {
	eb := eventbus.EBus
	eb.UnsubscribeModule(lvmComp)
}
