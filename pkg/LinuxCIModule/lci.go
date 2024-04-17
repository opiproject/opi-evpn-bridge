// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package linuxcimodule is the main package of the application
package linuxcimodule

import (
	"context"

	// "io/ioutil"
	"log"
	"math"
	"path"
	"time"

	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	// "gopkg.in/yaml.v2"
)

// ModulelciHandler interface
type ModulelciHandler struct{}

const lciComp string = "lci"

// HandleEvent handle the registered events
func (h *ModulelciHandler) HandleEvent(eventType string, objectData *eventbus.ObjectData) {
	switch eventType {
	case "bridge-port":
		log.Printf("LCI recevied %s %s\n", eventType, objectData.Name)
		handlebp(objectData)
	default:
		log.Printf("LCI: error: Unknown event type %s", eventType)
	}
}

// handlebp  handle the bridge port functionality
func handlebp(objectData *eventbus.ObjectData) {
	var comp common.Component
	BP, err := infradb.GetBP(objectData.Name)
	if err != nil {
		log.Printf("LCI : GetBP error: %s\n", err)
		return
	}
	if objectData.ResourceVersion != BP.ResourceVersion {
		log.Printf("LVM: Mismatch in resoruce version %+v\n and bp resource version %+v\n", objectData.ResourceVersion, BP.ResourceVersion)
		comp.Name = lciComp
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
	if len(BP.Status.Components) != 0 {
		for i := 0; i < len(BP.Status.Components); i++ {
			if BP.Status.Components[i].Name == "lci" {
				comp = BP.Status.Components[i]
			}
		}
	}
	if BP.Status.BPOperStatus != infradb.BridgePortOperStatusToBeDeleted {
		status := setUpBp(BP)
		comp.Name = lciComp
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
		log.Printf("LCI: %+v \n", comp)
		err := infradb.UpdateBPStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, BP.Metadata, comp)
		if err != nil {
			log.Printf("error in updating bp status: %s\n", err)
		}
	} else {
		status := tearDownBp(BP)
		comp.Name = lciComp
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
		log.Printf("LCI: %+v \n", comp)
		err := infradb.UpdateBPStatus(objectData.Name, objectData.ResourceVersion, objectData.NotificationID, nil, comp)
		if err != nil {
			log.Printf("error in updating bp status: %s\n", err)
		}
	}
}

// setUpBp sets up the bridge port
func setUpBp(bp *infradb.BridgePort) bool {
	resourceID := path.Base(bp.Name)
	bridge, err := nlink.LinkByName(ctx, "br-tenant")
	if err != nil {
		log.Printf("LCI: Unable to find key br-tenant\n")
		return false
	}
	iface, err := nlink.LinkByName(ctx, resourceID)
	if err != nil {
		log.Printf("LCI: Unable to find key %s\n", resourceID)
		return false
	}
	if err := nlink.LinkSetMaster(ctx, iface, bridge); err != nil {
		log.Printf("LCI: Failed to add iface to bridge: %v", err)
		return false
	}
	for _, bridgeRefName := range bp.Spec.LogicalBridges {
		BrObj, err := infradb.GetLB(bridgeRefName)
		if err != nil {
			log.Printf("LCI: unable to find key %s and error is %v", bridgeRefName, err)
			return false
		}
		if BrObj.Spec.VlanID > math.MaxUint16 {
			log.Printf("LVM : VlanID %v value passed in Logical Bridge create is greater than 16 bit value\n", BrObj.Spec.VlanID)
			return false
		}
		//TODO: Update opi-api to change vlanid to uint16 in LogiclaBridge
		vid := uint16(BrObj.Spec.VlanID)
		switch bp.Spec.Ptype {
		case infradb.Access:
			if err := nlink.BridgeVlanAdd(ctx, iface, vid, true, true, false, false); err != nil {
				log.Printf("Failed to add vlan to bridge: %v", err)
				return false
			}
		case infradb.Trunk:
			// Example: bridge vlan add dev eth2 vid 20
			if err := nlink.BridgeVlanAdd(ctx, iface, vid, false, false, false, false); err != nil {
				log.Printf("Failed to add vlan to bridge: %v", err)
				return false
			}
		default:
			log.Printf("Only ACCESS or TRUNK supported and not (%d)", bp.Spec.Ptype)
			return false
		}
	}
	if err := nlink.LinkSetUp(ctx, iface); err != nil {
		log.Printf("Failed to up iface link: %v", err)
		return false
	}
	return true
}

// tearDownBp tears down a bridge port
func tearDownBp(bp *infradb.BridgePort) bool {
	resourceID := path.Base(bp.Name)
	iface, err := nlink.LinkByName(ctx, resourceID)
	if err != nil {
		log.Printf("LCI: Unable to find key %s\n", resourceID)
		return false
	}
	if err := nlink.LinkSetDown(ctx, iface); err != nil {
		log.Printf("LCI: Failed to down link: %v", err)
		return false
	}
	for _, bridgeRefName := range bp.Spec.LogicalBridges {
		BrObj, err := infradb.GetLB(bridgeRefName)
		if err != nil {
			log.Printf("LCI: unable to find key %s and error is %v", bridgeRefName, err)
			return false
		}
		if BrObj.Spec.VlanID > math.MaxUint16 {
			log.Printf("LVM : VlanID %v value passed in Logical Bridge create is greater than 16 bit value\n", BrObj.Spec.VlanID)
			return false
		}
		//TODO: Update opi-api to change vlanid to uint16 in LogiclaBridge
		vid := uint16(BrObj.Spec.VlanID)
		if err := nlink.BridgeVlanDel(ctx, iface, vid, true, true, false, false); err != nil {
			log.Printf("LCI: Failed to delete vlan to bridge: %v", err)
			return false
		}
	}
	if err := nlink.LinkDel(ctx, iface); err != nil {
		log.Printf("Failed to delete link: %v", err)
		return false
	}
	return true
}

var ctx context.Context
var nlink utils.Netlink

// Init initializes the config and  subscribers
func Init() {
	eb := eventbus.EBus
	for _, subscriberConfig := range config.GlobalConfig.Subscribers {
		if subscriberConfig.Name == "lci" {
			for _, eventType := range subscriberConfig.Events {
				eb.StartSubscriber(subscriberConfig.Name, eventType, subscriberConfig.Priority, &ModulelciHandler{})
			}
		}
	}
	ctx = context.Background()
	nlink = utils.NewNetlinkWrapper()
}
