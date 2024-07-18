// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package infradb exposes the interface for the manipulation of the api objects
package infradb

import (
	"errors"
	"log"
	"sync"
	"time"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/taskmanager"
	"github.com/opiproject/opi-evpn-bridge/pkg/storage"
	"github.com/philippgille/gokv"
)

var infradb *InfraDB
var globalLock sync.Mutex

// InfraDB structure
type InfraDB struct {
	client gokv.Store
}

var (
	// ErrKeyNotFound error for missing key
	ErrKeyNotFound = errors.New("key not found")
	// ErrComponentNotFound error for missing component
	ErrComponentNotFound = errors.New("component not found")
	// ErrVrfNotFound error for missing vrf
	ErrVrfNotFound = errors.New("the referenced VRF has not been found")
	// ErrLogicalBridgeNotFound error for missing logical bridge
	ErrLogicalBridgeNotFound = errors.New("the referenced Logical Bridge has not been found")
	// ErrVrfNotEmpty vrf is not empty
	ErrVrfNotEmpty = errors.New("the VRF is not empty")
	// ErrLogicalBridgeNotEmpty logical bridge is not empty
	ErrLogicalBridgeNotEmpty = errors.New("the LogicalBridge is not empty")
	// ErrRoutingTableInUse routing table is in use
	ErrRoutingTableInUse = errors.New("the routing table is already in use")
	// ErrVniInUse vni is in use
	ErrVniInUse = errors.New("the VNI is already in use")
	// Add more error constants as needed
)

// NewInfraDB initializes an InfraDB object
func NewInfraDB(address string, dbtype string) error {
	store, err := storage.NewStore(dbtype, address)
	if err != nil {
		log.Println(err)
		return err
	}

	infradb = &InfraDB{
		client: store.GetClient(),
	}
	return nil
}

// Close closes a infradb connection to the DB
func Close() error {
	return infradb.client.Close()
}

// CreateLB creates an infradb logical bridge object
func CreateLB(lb *LogicalBridge) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	vpns := make(map[uint32]bool)

	subscribers := eventbus.EBus.GetSubscribers("logical-bridge")
	if len(subscribers) == 0 {
		log.Println("CreateLB(): No subscribers for Logical Bridge objects")
		return errors.New("no subscribers found for logical bridge")
	}

	log.Printf("CreateLB(): Create Logical Bridge: %+v\n", lb)

	// Check if VNI is already used
	if lb.Spec.Vni != nil {
		found, err := infradb.client.Get("vpns", &vpns)
		if err != nil {
			log.Println(err)
			return err
		}

		if !found {
			vpns[*lb.Spec.Vni] = false
		} else {
			_, ok := vpns[*lb.Spec.Vni]
			if ok {
				log.Printf("CreateLB(): VNI already in use: %+v\n", lb.Spec.Vni)
				return ErrVniInUse
			}
			vpns[*lb.Spec.Vni] = false
		}
	}

	err := infradb.client.Set(lb.Name, lb)
	if err != nil {
		log.Println(err)
		return err
	}

	// Store VNI to DB in the vpns map
	if lb.Spec.Vni != nil {
		err = infradb.client.Set("vpns", &vpns)
		if err != nil {
			log.Println(err)
			return err
		}
	}

	// Add the New Created Logical Bridge to the "lbs" map
	lbs := make(map[string]bool)
	_, err = infradb.client.Get("lbs", &lbs)
	if err != nil {
		log.Println(err)
		return err
	}
	// The reason that we use a map and not a list is
	// because in the delete case we can delete the LB from the
	// map by just using the name. No need to iterate the whole list until
	// we find the LB and then delete it.
	lbs[lb.Name] = false
	err = infradb.client.Set("lbs", &lbs)
	if err != nil {
		log.Println(err)
		return err
	}

	taskmanager.TaskMan.CreateTask(lb.Name, "logical-bridge", lb.ResourceVersion, subscribers)

	return nil
}

// DeleteLB deletes a logical bridge infradb object
func DeleteLB(name string) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	subscribers := eventbus.EBus.GetSubscribers("logical-bridge")
	if len(subscribers) == 0 {
		log.Println("DeleteLB(): No subscribers for Logical Bridge objects")
		return errors.New("no subscribers found for logical bridge")
	}

	lb := LogicalBridge{}
	found, err := infradb.client.Get(name, &lb)
	if err != nil {
		log.Println(err)
		return err
	}
	if !found {
		return ErrKeyNotFound
	}

	if lb.Svi != "" {
		log.Printf("DeleteLB(): Can not delete Logical Bridge %+v. Associated with SVI interfaces", lb.Name)
		return ErrLogicalBridgeNotEmpty
	}

	if len(lb.BridgePorts) != 0 || len(lb.MacTable) != 0 {
		log.Printf("DeleteLB(): Can not delete Logical Bridge %+v. Associated with Bridge Ports", lb.Name)
		return ErrLogicalBridgeNotEmpty
	}

	for i := range subscribers {
		lb.Status.Components[i].CompStatus = common.ComponentStatusPending
	}
	lb.ResourceVersion = generateVersion()
	lb.Status.LBOperStatus = LogicalBridgeOperStatusToBeDeleted

	err = infradb.client.Set(lb.Name, lb)
	if err != nil {
		return err
	}

	taskmanager.TaskMan.CreateTask(lb.Name, "logical-bridge", lb.ResourceVersion, subscribers)

	return nil
}

// GetLB returns an infradb logical bridge object
func GetLB(name string) (*LogicalBridge, error) {
	globalLock.Lock()
	defer globalLock.Unlock()

	lb := LogicalBridge{}
	found, err := infradb.client.Get(name, &lb)

	if !found {
		return &lb, ErrKeyNotFound
	}
	return &lb, err
}

// GetAllLBs returns a list of logical bridges from the DB
func GetAllLBs() ([]*LogicalBridge, error) {
	globalLock.Lock()
	defer globalLock.Unlock()

	lbs := []*LogicalBridge{}
	lbsMap := make(map[string]bool)
	found, err := infradb.client.Get("lbs", &lbsMap)

	if err != nil {
		log.Println(err)
		return nil, err
	}

	if !found {
		log.Println("GetAllLogicalBridges(): No Logical Bridges have been found")
		return nil, ErrKeyNotFound
	}

	for key := range lbsMap {
		lb := &LogicalBridge{}
		found, err := infradb.client.Get(key, lb)

		if err != nil {
			log.Printf("GetAllLogicalBridges(): Failed to get the Logical Bridge %s from store: %v", key, err)
			return nil, err
		}

		if !found {
			log.Printf("GetAllLogicalBridges():Logical Bridge %s not found", key)
			return nil, ErrKeyNotFound
		}
		lbs = append(lbs, lb)
	}

	return lbs, nil
}

// UpdateLB updates a logical bridge infradb object
func UpdateLB(lb *LogicalBridge) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	subscribers := eventbus.EBus.GetSubscribers("logical-bridge")
	if len(subscribers) == 0 {
		log.Println("UpdateLB(): No subscribers for Logical Bridge objects")
		return errors.New("no subscribers found for logical bridge")
	}

	err := infradb.client.Set(lb.Name, lb)
	if err != nil {
		log.Println(err)
		return err
	}

	taskmanager.TaskMan.CreateTask(lb.Name, "logical-bridge", lb.ResourceVersion, subscribers)

	return nil
}
func removeVniFromVpns(vni uint32) error {
	vpns := make(map[uint32]bool)
	if vni != 0 {
		found, err := infradb.client.Get("vpns", &vpns)
		if err != nil {
			return err
		}
		if !found {
			return ErrKeyNotFound
		}
		delete(vpns, vni)

		err = infradb.client.Set("vpns", &vpns)
		if err != nil {
			return err
		}
	}
	return nil
}

// UpdateLBStatus updates the status of logical bridge object based on the component report
// nolint: funlen
//
//gocognit:ignore
func UpdateLBStatus(name string, resourceVersion string, notificationID string, lbMeta *LogicalBridgeMetadata, component common.Component) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	var allCompSuccess bool

	// When we get an error from an operation to the Database then we just return it. The
	// Task manager will just expire the task and retry.
	lb := LogicalBridge{}
	found, err := infradb.client.Get(name, &lb)
	if err != nil {
		log.Println(err)
		return err
	}

	if !found {
		// No Logical Bridge object has been found in the database so we will instruct TaskManager to drop the Task that is related with this status update.
		taskmanager.TaskMan.StatusUpdated(name, "logical-bridge", lb.ResourceVersion, notificationID, true, &component)
		log.Printf("UpdateLBStatus(): No Logical Bridge object has been found in DB with Name %s\n", name)
		return nil
	}

	if lb.ResourceVersion != resourceVersion {
		// Logical Bridge object in the database with different resourceVersion so we will instruct TaskManager to drop the Task that is related with this status update.
		taskmanager.TaskMan.StatusUpdated(lb.Name, "logical-bridge", lb.ResourceVersion, notificationID, true, &component)
		log.Printf("UpdateLBStatus(): Invalid resourceVersion %s for Logical Bridge %+v\n", resourceVersion, lb)
		return nil
	}

	if component.Replay {
		// One of the components has requested a replay of the DB.
		// The task related to the status update will be dropped.
		log.Printf("UpdateLBStatus(): Component %s has requested a replay\n", component.Name)
		taskmanager.TaskMan.StatusUpdated(lb.Name, "logical-bridge", lb.ResourceVersion, notificationID, true, &component)
		go startReplayProcedure(component.Name)
		return nil
	}

	// Set the state of the component
	lb.setComponentState(component)

	// Check if all the components are in Success state
	allCompSuccess = lb.checkForAllSuccess()

	// Parse the Metadata that has been sent from the Component
	lb.parseMeta(lbMeta)

	// Is it ok to delete an object before we update the last component status to success ?
	if allCompSuccess {
		if lb.Status.LBOperStatus == LogicalBridgeOperStatusToBeDeleted {
			err = infradb.client.Delete(lb.Name)
			if err != nil {
				log.Println(err)
				return err
			}

			// Delete VNI from the VPN map
			if lb.Spec.Vni != nil {
				err = removeVniFromVpns(*lb.Spec.Vni)
				if err != nil {
					return err
				}
			}

			lbs := make(map[string]bool)
			found, err = infradb.client.Get("lbs", &lbs)
			if err != nil {
				log.Println(err)
				return err
			}
			if !found {
				log.Println("UpdateLBStatus(): No Logical Bridges have been found")
				return ErrKeyNotFound
			}

			delete(lbs, lb.Name)
			err = infradb.client.Set("lbs", &lbs)
			if err != nil {
				log.Println(err)
				return err
			}

			log.Printf("UpdateLBStatus(): Logical Bridge %s has been deleted\n", name)
		} else {
			lb.Status.LBOperStatus = LogicalBridgeOperStatusUp
			err = infradb.client.Set(lb.Name, lb)
			if err != nil {
				log.Println(err)
				return err
			}
			log.Printf("UpdateLBStatus(): Logical Bridge %s has been updated: %+v\n", name, lb)
		}
	} else {
		err = infradb.client.Set(lb.Name, lb)
		if err != nil {
			log.Println(err)
			return err
		}
		log.Printf("UpdateLBStatus(): Logical Bridge %s has been updated: %+v\n", name, lb)
	}

	taskmanager.TaskMan.StatusUpdated(lb.Name, "logical-bridge", lb.ResourceVersion, notificationID, false, &component)

	return nil
}

// CreateBP creates an infradb bridge port object
func CreateBP(bp *BridgePort) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	subscribers := eventbus.EBus.GetSubscribers("bridge-port")
	if len(subscribers) == 0 {
		log.Println("CreateBP(): No subscribers for Bridge Port objects")
		return errors.New("no subscribers found for bridge port")
	}

	// Dimitris: Do I need to add here a check for MAC uniquness in of BP ?
	// The way to do this is to create a MAP of MACs and store it in the DB
	// and then check if MAC exist in this MAP everytime a new BP gets created.

	log.Printf("CreateBP(): Create Bridge Port: %+v\n", bp)

	// If Transparent Trunk then all the Logical Bridges are included by default
	if bp.TransparentTrunk {
		lbs := make(map[string]bool)
		found, err := infradb.client.Get("lbs", &lbs)
		if err != nil {
			log.Println(err)
			return err
		}
		if !found {
			log.Println("CreateBP(): No Logical Bridges have been found")
			return ErrKeyNotFound
		}

		for lbName := range lbs {
			bp.Spec.LogicalBridges = append(bp.Spec.LogicalBridges, lbName)
		}
	}

	// Get Logical Bridge infraDB objects
	// Fill up the Vlans list of the infraDB Bridge Port object
	// Add Bridge Port reference and save the Logical Bridge object back to DB
	for _, lbName := range bp.Spec.LogicalBridges {
		lb := LogicalBridge{}
		found, err := infradb.client.Get(lbName, &lb)
		if err != nil {
			log.Println(err)
			return err
		}
		if !found {
			log.Printf("CreateBP(): The Logical Bridge with name %+v has not been found\n", lbName)
			return ErrLogicalBridgeNotFound
		}
		bp.Vlans = append(bp.Vlans, &lb.Spec.VlanID)

		// Store Bridge Port reference to the Logical Bridge object
		err = lb.AddBridgePort(bp.Name, bp.Spec.MacAddress.String())
		if err != nil {
			log.Printf("CreateBP(): Error: %+v", err)
			return err
		}

		// Save Logical Bridge object back to DB
		err = infradb.client.Set(lb.Name, lb)
		if err != nil {
			log.Println(err)
			return err
		}
	}

	// Store Bridge Port object to Database
	err := infradb.client.Set(bp.Name, bp)
	if err != nil {
		log.Println(err)
		return err
	}

	// Add the New Created Bridge Port to the "bps" map
	bps := make(map[string]bool)
	_, err = infradb.client.Get("bps", &bps)
	if err != nil {
		log.Println(err)
		return err
	}
	// The reason that we use a map and not a list is
	// because in the delete case we can delete the Bridge Port from the
	// map by just using the name. No need to iterate the whole list until
	// we find the Bridge port and then delete it.
	bps[bp.Name] = false
	err = infradb.client.Set("bps", &bps)
	if err != nil {
		log.Println(err)
		return err
	}

	taskmanager.TaskMan.CreateTask(bp.Name, "bridge-port", bp.ResourceVersion, subscribers)

	return nil
}

// DeleteBP deletes a bridge port infradb object
func DeleteBP(name string) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	subscribers := eventbus.EBus.GetSubscribers("bridge-port")
	if len(subscribers) == 0 {
		log.Println("DeleteBP(): No subscribers for Bridge Port objects")
		return errors.New("no subscribers found for bridge port")
	}

	bp := BridgePort{}
	found, err := infradb.client.Get(name, &bp)
	if err != nil {
		return err
	}
	if !found {
		return ErrKeyNotFound
	}

	for i := range subscribers {
		bp.Status.Components[i].CompStatus = common.ComponentStatusPending
	}
	bp.ResourceVersion = generateVersion()
	bp.Status.BPOperStatus = BridgePortOperStatusToBeDeleted

	err = infradb.client.Set(bp.Name, bp)
	if err != nil {
		return err
	}

	taskmanager.TaskMan.CreateTask(bp.Name, "bridge-port", bp.ResourceVersion, subscribers)

	return nil
}

// GetBP returns an infradb bridge port object
func GetBP(name string) (*BridgePort, error) {
	globalLock.Lock()
	defer globalLock.Unlock()

	bp := BridgePort{}
	found, err := infradb.client.Get(name, &bp)

	if !found {
		return &bp, ErrKeyNotFound
	}
	return &bp, err
}

// GetAllBPs returns a list of bridge ports from the DB
func GetAllBPs() ([]*BridgePort, error) {
	globalLock.Lock()
	defer globalLock.Unlock()

	bps := []*BridgePort{}
	bpsMap := make(map[string]bool)
	found, err := infradb.client.Get("bps", &bpsMap)

	if err != nil {
		log.Println(err)
		return nil, err
	}

	if !found {
		log.Println("GetAllBPs(): No Bridge Ports have been found")
		return nil, ErrKeyNotFound
	}

	for key := range bpsMap {
		bp := &BridgePort{}
		found, err := infradb.client.Get(key, bp)

		if err != nil {
			log.Printf("GetAllBPs(): Failed to get the Bridge Port %s from store: %v", key, err)
			return nil, err
		}

		if !found {
			log.Printf("GetAllBPs(): Bridge Port %s not found", key)
			return nil, ErrKeyNotFound
		}
		bps = append(bps, bp)
	}

	return bps, nil
}

// UpdateBP updates a bridge port infradb object
func UpdateBP(bp *BridgePort) error {
	// Note: The update functions for all the objects need to be revisited
	// The implementaation currently is not correct but due to low priority
	// will be refactored in the future.
	globalLock.Lock()
	defer globalLock.Unlock()

	subscribers := eventbus.EBus.GetSubscribers("bridge-port")
	if len(subscribers) == 0 {
		log.Println("UpdateBP(): No subscribers for Bridge Port objects")
		return errors.New("no subscribers found for bridge port")
	}

	err := infradb.client.Set(bp.Name, bp)
	if err != nil {
		log.Println(err)
		return err
	}

	taskmanager.TaskMan.CreateTask(bp.Name, "bridge-port", bp.ResourceVersion, subscribers)

	return nil
}

// UpdateBPStatus updates the status of bridge port object based on the component report
// nolint: funlen
//
//gocognit:ignore
func UpdateBPStatus(name string, resourceVersion string, notificationID string, bpMeta *BridgePortMetadata, component common.Component) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	var allCompSuccess bool

	// When we get an error from an operation to the Database then we just return it. The
	// Task manager will just expire the task and retry.
	bp := BridgePort{}
	found, err := infradb.client.Get(name, &bp)
	if err != nil {
		log.Println(err)
		return err
	}

	if !found {
		// No Bridge Port object has been found in the database so we will instruct TaskManager to drop the Task that is related with this status update.
		taskmanager.TaskMan.StatusUpdated(name, "bridge-port", bp.ResourceVersion, notificationID, true, &component)
		log.Printf("UpdateBPStatus(): No Bridge Port object has been found in DB with Name %s\n", name)
		return nil
	}

	if bp.ResourceVersion != resourceVersion {
		// Bridge Port object in the database with different resourceVersion so we will instruct TaskManager to drop the Task that is related with this status update.
		taskmanager.TaskMan.StatusUpdated(bp.Name, "bridge-port", bp.ResourceVersion, notificationID, true, &component)
		log.Printf("UpdateBPStatus(): Invalid resourceVersion %s for Bridge Port %+v\n", resourceVersion, bp)
		return nil
	}

	if component.Replay {
		// One of the components has requested a replay of the DB.
		// The task related to the status update will be dropped.
		log.Printf("UpdateBPStatus(): Component %s has requested a replay\n", component.Name)
		taskmanager.TaskMan.StatusUpdated(bp.Name, "bridge-port", bp.ResourceVersion, notificationID, true, &component)
		go startReplayProcedure(component.Name)
		return nil
	}

	// Set the state of the component
	bp.setComponentState(component)

	// Check if all the components are in Success state
	allCompSuccess = bp.checkForAllSuccess()

	// Parse the Metadata that has been sent from the Component
	bp.parseMeta(bpMeta)

	// Is it ok to delete an object before we update the last component status to success ?
	// Take care of deleting the references to the LB  objects after the BP has been successfully deleted
	if allCompSuccess {
		if bp.Status.BPOperStatus == SviOperStatusToBeDeleted {
			// Delete the references from Logical Bridge objects
			for _, lbName := range bp.Spec.LogicalBridges {
				lb := LogicalBridge{}
				_, err := infradb.client.Get(lbName, &lb)
				if err != nil {
					log.Println(err)
					return err
				}

				// Store Bridge Port reference to the Logical Bridge object
				err = lb.DeleteBridgePort(bp.Name, bp.Spec.MacAddress.String())
				if err != nil {
					log.Printf("UpdateBPStatus(): Error: %+v", err)
					return err
				}

				// Save Logical Bridge object back to DB
				err = infradb.client.Set(lb.Name, lb)
				if err != nil {
					log.Println(err)
					return err
				}
			}

			// Delete the Bridge Port object from the DB
			err = infradb.client.Delete(bp.Name)
			if err != nil {
				log.Println(err)
				return err
			}

			// Delete the Bridge Port from the bps map
			bps := make(map[string]bool)
			found, err = infradb.client.Get("bps", &bps)
			if err != nil {
				log.Println(err)
				return err
			}
			if !found {
				log.Println("UpdateBPStatus(): No Bridge Ports have been found")
				return ErrKeyNotFound
			}

			delete(bps, bp.Name)
			err = infradb.client.Set("bps", &bps)
			if err != nil {
				log.Println(err)
				return err
			}

			log.Printf("UpdateBPStatus(): Bridge Port %s has been deleted\n", name)
		} else {
			bp.Status.BPOperStatus = BridgePortOperStatusUp
			err = infradb.client.Set(bp.Name, bp)
			if err != nil {
				log.Println(err)
				return err
			}
			log.Printf("UpdateBPStatus(): Bridge Port %s has been updated: %+v\n", name, bp)
		}
	} else {
		err = infradb.client.Set(bp.Name, bp)
		if err != nil {
			log.Println(err)
			return err
		}
		log.Printf("UpdateBPStatus(): Bridge Port %s has been updated: %+v\n", name, bp)
	}

	taskmanager.TaskMan.StatusUpdated(bp.Name, "bridge-port", bp.ResourceVersion, notificationID, false, &component)

	return nil
}

// CreateVrf creates an infradb vrf object
func CreateVrf(vrf *Vrf) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	vpns := make(map[uint32]bool)

	subscribers := eventbus.EBus.GetSubscribers("vrf")
	if len(subscribers) == 0 {
		log.Println("CreateVrf(): No subscribers for Vrf objects")
		return errors.New("no subscribers found for vrf")
	}

	log.Printf("CreateVrf(): Create Vrf: %+v\n", vrf)

	// TODO: Move the check for VNI in a common place
	// and use that comomn code also for checking the LB vni
	// Check if VNI is already used
	if vrf.Spec.Vni != nil {
		found, err := infradb.client.Get("vpns", &vpns)
		if err != nil {
			log.Println(err)
			return err
		}

		if !found {
			vpns[*vrf.Spec.Vni] = false
		} else {
			_, ok := vpns[*vrf.Spec.Vni]
			if ok {
				log.Printf("CreateVrf(): VNI already in use: %+v\n", *vrf.Spec.Vni)
				return ErrVniInUse
			}
			vpns[*vrf.Spec.Vni] = false
		}
	}

	err := infradb.client.Set(vrf.Name, vrf)
	if err != nil {
		log.Println(err)
		return err
	}

	// Store VNI to DB in the vpns map
	if vrf.Spec.Vni != nil {
		err = infradb.client.Set("vpns", &vpns)
		if err != nil {
			log.Println(err)
			return err
		}
	}

	// Add the New Created VRF to the "vrfs" map
	vrfs := make(map[string]bool)
	_, err = infradb.client.Get("vrfs", &vrfs)
	if err != nil {
		log.Println(err)
		return err
	}
	// The reason that we use a map and not a list is
	// because in the delete case we can delete the vrf from the
	// map by just using the name. No need to iterate the whole list until
	// we find the vrf and then delete it.
	vrfs[vrf.Name] = false
	err = infradb.client.Set("vrfs", &vrfs)
	if err != nil {
		log.Println(err)
		return err
	}

	taskmanager.TaskMan.CreateTask(vrf.Name, "vrf", vrf.ResourceVersion, subscribers)

	return nil
}

// DeleteVrf deletes a vrf infradb object
func DeleteVrf(name string) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	subscribers := eventbus.EBus.GetSubscribers("vrf")
	if len(subscribers) == 0 {
		log.Println("DeleteVrf(): No subscribers for Vrf objects")
		return errors.New("no subscribers found for vrf")
	}

	vrf := Vrf{}
	found, err := infradb.client.Get(name, &vrf)
	if err != nil {
		return err
	}
	if !found {
		return ErrKeyNotFound
	}

	if len(vrf.Svis) != 0 {
		log.Printf("DeleteVrf(): Can not delete VRF %+v. Associated with SVI interfaces", vrf.Name)
		return ErrVrfNotEmpty
	}

	for i := range subscribers {
		vrf.Status.Components[i].CompStatus = common.ComponentStatusPending
	}
	vrf.ResourceVersion = generateVersion()
	vrf.Status.VrfOperStatus = VrfOperStatusToBeDeleted

	err = infradb.client.Set(vrf.Name, vrf)
	if err != nil {
		return err
	}

	taskmanager.TaskMan.CreateTask(vrf.Name, "vrf", vrf.ResourceVersion, subscribers)

	return nil
}

// GetVrf returns an infradb vrf object
func GetVrf(name string) (*Vrf, error) {
	globalLock.Lock()
	defer globalLock.Unlock()

	vrf := Vrf{}
	found, err := infradb.client.Get(name, &vrf)

	if !found {
		return &vrf, ErrKeyNotFound
	}
	return &vrf, err
}

// GetAllVrfs returns a list of svis from the DB
func GetAllVrfs() ([]*Vrf, error) {
	globalLock.Lock()
	defer globalLock.Unlock()

	vrfs := []*Vrf{}
	vrfsMap := make(map[string]bool)
	found, err := infradb.client.Get("vrfs", &vrfsMap)

	if err != nil {
		log.Println(err)
		return nil, err
	}

	if !found {
		log.Println("GetAllVrfs(): No VRFs have been found")
		return nil, ErrKeyNotFound
	}

	for key := range vrfsMap {
		vrf := &Vrf{}
		found, err := infradb.client.Get(key, vrf)

		if err != nil {
			log.Printf("GetAllVrfs(): Failed to get the VRF %s from store: %v", key, err)
			return nil, err
		}

		if !found {
			log.Printf("GetAllVrfs(): VRF %s not found", key)
			return nil, ErrKeyNotFound
		}
		vrfs = append(vrfs, vrf)
	}

	return vrfs, nil
}

// UpdateVrf updates a vrf infradb object
func UpdateVrf(vrf *Vrf) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	subscribers := eventbus.EBus.GetSubscribers("vrf")
	if len(subscribers) == 0 {
		log.Println("CreateVrf(): No subscribers for Vrf objects")
		return errors.New("no subscribers found for vrf")
	}

	err := infradb.client.Set(vrf.Name, vrf)
	if err != nil {
		log.Println(err)
		return err
	}

	taskmanager.TaskMan.CreateTask(vrf.Name, "vrf", vrf.ResourceVersion, subscribers)

	return nil
}

// UpdateVrfStatus updates the status of vrf object based on the component report
// nolint: funlen, gocognit
func UpdateVrfStatus(name string, resourceVersion string, notificationID string, vrfMeta *VrfMetadata, component common.Component) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	var allCompSuccess bool

	// When we get an error from an operation to the Database then we just return it. The
	// Task manager will just expire the task and retry.
	vrf := Vrf{}
	found, err := infradb.client.Get(name, &vrf)
	if err != nil {
		log.Println(err)
		return err
	}

	if !found {
		// No VRF object has been found in the database so we will instruct TaskManager to drop the Task that is related with this status update.
		taskmanager.TaskMan.StatusUpdated(name, "vrf", vrf.ResourceVersion, notificationID, true, &component)
		log.Printf("UpdateVrfStatus(): No VRF object has been found in DB with Name %s\n", name)
		return nil
	}

	if vrf.ResourceVersion != resourceVersion {
		// VRF object in the database with different resourceVersion so we will instruct TaskManager to drop the Task that is related with this status update.
		taskmanager.TaskMan.StatusUpdated(vrf.Name, "vrf", vrf.ResourceVersion, notificationID, true, &component)
		log.Printf("UpdateVrfStatus(): Invalid resourceVersion %s for VRF %+v\n", resourceVersion, vrf)
		return nil
	}

	// Here we check if the component has asked for a replay of the DB to be taken place
	if component.Replay {
		// One of the components has requested a replay of the DB.
		// The task related to the status update will be dropped.
		log.Printf("UpdateVrfStatus(): Component %s has requested a replay\n", component.Name)
		taskmanager.TaskMan.StatusUpdated(vrf.Name, "vrf", vrf.ResourceVersion, notificationID, true, &component)
		go startReplayProcedure(component.Name)
		return nil
	}

	// Set the state of the component
	vrf.setComponentState(component)

	// Check if all the components are in Success state
	allCompSuccess = vrf.checkForAllSuccess()

	// Parse the Metadata that has been sent from the Component
	vrf.parseMeta(vrfMeta)

	// Is it ok to delete an object before we update the last component status to success ?
	if allCompSuccess {
		if vrf.Status.VrfOperStatus == VrfOperStatusToBeDeleted {
			err = infradb.client.Delete(vrf.Name)
			if err != nil {
				log.Println(err)
				return err
			}

			// Delete VNI from the VPN map
			if vrf.Spec.Vni != nil {
				err = removeVniFromVpns(*vrf.Spec.Vni)
				if err != nil {
					return err
				}
			}

			// Delete VRF from VRFs Map
			vrfs := make(map[string]bool)
			found, err = infradb.client.Get("vrfs", &vrfs)
			if err != nil {
				log.Println(err)
				return err
			}
			if !found {
				log.Println("UpdateVrfStatus(): No VRFs have been found")
				return ErrKeyNotFound
			}

			delete(vrfs, vrf.Name)
			err = infradb.client.Set("vrfs", &vrfs)
			if err != nil {
				log.Println(err)
				return err
			}

			log.Printf("UpdateVrfStatus(): VRF %s has been deleted\n", name)
		} else {
			vrf.Status.VrfOperStatus = VrfOperStatusUp
			err = infradb.client.Set(vrf.Name, vrf)
			if err != nil {
				log.Println(err)
				return err
			}
			log.Printf("UpdateVrfStatus(): VRF %s has been updated: %+v\n", name, vrf)
		}
	} else {
		err = infradb.client.Set(vrf.Name, vrf)
		if err != nil {
			log.Println(err)
			return err
		}
		log.Printf("UpdateVrfStatus(): VRF %s has been updated: %+v\n", name, vrf)
	}

	taskmanager.TaskMan.StatusUpdated(vrf.Name, "vrf", vrf.ResourceVersion, notificationID, false, &component)

	return nil
}

// CreateSvi creates an infradb svi object
func CreateSvi(svi *Svi) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	subscribers := eventbus.EBus.GetSubscribers("svi")
	if len(subscribers) == 0 {
		log.Println("CreateSvi(): No subscribers for SVI objects")
		return errors.New("no subscribers found for svi")
	}

	log.Printf("CreateSvi(): Create SVI: %+v\n", svi)

	// Checking if the VRF exists
	vrf := Vrf{}
	found, err := infradb.client.Get(svi.Spec.Vrf, &vrf)
	if err != nil {
		log.Println(err)
		return err
	}
	if !found {
		log.Printf("CreateSvi(): The VRF with name %+v has not been found\n", svi.Spec.Vrf)
		return ErrVrfNotFound
	}

	// Checking if the Logical Bridge exists
	lb := LogicalBridge{}
	found, err = infradb.client.Get(svi.Spec.LogicalBridge, &lb)
	if err != nil {
		log.Println(err)
		return err
	}
	if !found {
		log.Printf("CreateSvi(): The Logical Bridge with name %+v has not been found\n", svi.Spec.LogicalBridge)
		return ErrLogicalBridgeNotFound
	}

	// Store svi reference to the VRF object
	if err := vrf.AddSvi(svi.Name); err != nil {
		log.Println(err)
		return err
	}

	err = infradb.client.Set(vrf.Name, vrf)
	if err != nil {
		log.Println(err)
		return err
	}

	// Store svi reference to the Logical Bridge object
	if err := lb.AddSvi(svi.Name); err != nil {
		log.Println(err)
		return err
	}

	err = infradb.client.Set(lb.Name, lb)
	if err != nil {
		log.Println(err)
		return err
	}

	// Store SVI object to Database
	err = infradb.client.Set(svi.Name, svi)
	if err != nil {
		log.Println(err)
		return err
	}

	// Add the New Created SVI to the "svis" map
	svis := make(map[string]bool)
	_, err = infradb.client.Get("svis", &svis)
	if err != nil {
		log.Println(err)
		return err
	}
	// The reason that we use a map and not a list is
	// because in the delete case we can delete the SVI from the
	// map by just using the name. No need to iterate the whole list until
	// we find the SVI and then delete it.
	svis[svi.Name] = false
	err = infradb.client.Set("svis", &svis)
	if err != nil {
		log.Println(err)
		return err
	}

	taskmanager.TaskMan.CreateTask(svi.Name, "svi", svi.ResourceVersion, subscribers)

	return nil
}

// DeleteSvi deletes a svi infradb object
func DeleteSvi(name string) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	subscribers := eventbus.EBus.GetSubscribers("svi")
	if len(subscribers) == 0 {
		log.Println("DeleteSvi(): No subscribers for SVI objects")
		return errors.New("no subscribers found for svi")
	}

	svi := Svi{}
	found, err := infradb.client.Get(name, &svi)
	if err != nil {
		return err
	}
	if !found {
		return ErrKeyNotFound
	}

	for i := range subscribers {
		svi.Status.Components[i].CompStatus = common.ComponentStatusPending
	}
	svi.ResourceVersion = generateVersion()
	svi.Status.SviOperStatus = SviOperStatusToBeDeleted

	err = infradb.client.Set(svi.Name, svi)
	if err != nil {
		return err
	}

	taskmanager.TaskMan.CreateTask(svi.Name, "svi", svi.ResourceVersion, subscribers)

	return nil
}

// GetSvi returns an infradb svi object
func GetSvi(name string) (*Svi, error) {
	globalLock.Lock()
	defer globalLock.Unlock()

	svi := Svi{}
	found, err := infradb.client.Get(name, &svi)

	if !found {
		return &svi, ErrKeyNotFound
	}
	return &svi, err
}

// GetAllSvis returns a list of svis from the DB
func GetAllSvis() ([]*Svi, error) {
	globalLock.Lock()
	defer globalLock.Unlock()

	svis := []*Svi{}
	svisMap := make(map[string]bool)
	found, err := infradb.client.Get("svis", &svisMap)

	if err != nil {
		log.Println(err)
		return nil, err
	}

	if !found {
		log.Println("GetAllSvis(): No Svis have been found")
		return nil, ErrKeyNotFound
	}

	for key := range svisMap {
		svi := &Svi{}
		found, err := infradb.client.Get(key, svi)

		if err != nil {
			log.Printf("GetAllSvis(): Failed to get the SVI %s from store: %v", key, err)
			return nil, err
		}

		if !found {
			log.Printf("GetAllSvis(): SVI %s not found", key)
			return nil, ErrKeyNotFound
		}
		svis = append(svis, svi)
	}

	return svis, nil
}

// DeleteAllResources deletes all components from infradb
func DeleteAllResources() error {
	duration := 10 * time.Second
	bps, _ := GetAllBPs()
	for _, bp := range bps {
		err := DeleteBP(bp.Name)
		if err != nil {
			return err
		}
	}
	startTime := time.Now()
	for {
		b, _ := GetAllBPs()
		if len(b) == 0 {
			break
		}
		if time.Since(startTime) > duration {
			return errors.New("failed to delete BridgePorts")
		}
	}
	svis, _ := GetAllSvis()
	for _, svi := range svis {
		err := DeleteSvi(svi.Name)
		if err != nil {
			return err
		}
	}
	startTime = time.Now()
	for {
		s, _ := GetAllSvis()
		if len(s) == 0 {
			break
		}
		if time.Since(startTime) > duration {
			return errors.New("failed to delete svis")
		}
	}
	vrfs, _ := GetAllVrfs()
	for _, vrf := range vrfs {
		err := DeleteVrf(vrf.Name)
		if err != nil {
			return err
		}
	}
	startTime = time.Now()
	for {
		v, _ := GetAllVrfs()
		if len(v) == 0 {
			break
		}
		if time.Since(startTime) > duration {
			return errors.New("failed to delete vrfs")
		}
	}
	lbs, _ := GetAllLBs()
	for _, lb := range lbs {
		err := DeleteLB(lb.Name)
		if err != nil {
			return err
		}
	}
	startTime = time.Now()
	for {
		l, _ := GetAllLBs()
		if len(l) == 0 {
			break
		}
		if time.Since(startTime) > duration {
			return errors.New("failed to delete LogicalBridges")
		}
	}
	return nil
}

// UpdateSvi updates a svi infradb object
func UpdateSvi(svi *Svi) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	subscribers := eventbus.EBus.GetSubscribers("svi")
	if len(subscribers) == 0 {
		log.Println("UpdateSvi(): No subscribers for SVI objects")
		return errors.New("no subscribers found for svi")
	}

	err := infradb.client.Set(svi.Name, svi)
	if err != nil {
		log.Println(err)
		return err
	}

	taskmanager.TaskMan.CreateTask(svi.Name, "svi", svi.ResourceVersion, subscribers)

	return nil
}

// UpdateSviStatus updates the status of svi object based on the component report
// nolint: funlen
//
//gocognit:ignore
func UpdateSviStatus(name string, resourceVersion string, notificationID string, sviMeta *SviMetadata, component common.Component) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	var allCompSuccess bool

	// When we get an error from an operation to the Database then we just return it. The
	// Task manager will just expire the task and retry.
	svi := Svi{}
	found, err := infradb.client.Get(name, &svi)
	if err != nil {
		log.Println(err)
		return err
	}

	if !found {
		// No Svi object has been found in the database so we will instruct TaskManager to drop the Task that is related with this status update.
		taskmanager.TaskMan.StatusUpdated(name, "svi", svi.ResourceVersion, notificationID, true, &component)
		log.Printf("UpdateSviStatus(): No SVI object has been found in DB with Name %s\n", name)
		return nil
	}

	if svi.ResourceVersion != resourceVersion {
		// Svi object in the database with different resourceVersion so we will instruct TaskManager to drop the Task that is related with this status update.
		taskmanager.TaskMan.StatusUpdated(svi.Name, "svi", svi.ResourceVersion, notificationID, true, &component)
		log.Printf("UpdateSviStatus(): Invalid resourceVersion %s for SVI %+v\n", resourceVersion, svi)
		return nil
	}

	if component.Replay {
		// One of the components has requested a replay of the DB.
		// The task related to the status update will be dropped.
		log.Printf("UpdateSviStatus(): Component %s has requested a replay\n", component.Name)
		taskmanager.TaskMan.StatusUpdated(svi.Name, "svi", svi.ResourceVersion, notificationID, true, &component)
		go startReplayProcedure(component.Name)
		return nil
	}

	// Set the state of the component
	svi.setComponentState(component)

	// Check if all the components are in Success state
	allCompSuccess = svi.checkForAllSuccess()

	// Parse the Metadata that has been sent from the Component
	svi.parseMeta(sviMeta)

	// Is it ok to delete an object before we update the last component status to success ?
	// Take care of deleting the references to the LB and VRF objects after the SVI has been successfully deleted
	if allCompSuccess {
		if svi.Status.SviOperStatus == SviOperStatusToBeDeleted {
			// Delete the references from VRF and Logical Bridge objects

			// Get the dependent VRF object
			vrf := Vrf{}
			_, err := infradb.client.Get(svi.Spec.Vrf, &vrf)
			if err != nil {
				log.Println(err)
				return err
			}

			// Get the dependent Logical Bridge object
			lb := LogicalBridge{}
			_, err = infradb.client.Get(svi.Spec.LogicalBridge, &lb)
			if err != nil {
				log.Println(err)
				return err
			}

			// Delete the referenced SVI from the VRF and store the VRF to the DB
			if err := vrf.DeleteSvi(svi.Name); err != nil {
				log.Println(err)
				return err
			}

			err = infradb.client.Set(vrf.Name, vrf)
			if err != nil {
				log.Println(err)
				return err
			}

			// Delete the referenced SVI from the Logical Bridge and store the Logical Bridge to the DB
			if err := lb.DeleteSvi(svi.Name); err != nil {
				log.Println(err)
				return err
			}

			err = infradb.client.Set(lb.Name, lb)
			if err != nil {
				log.Println(err)
				return err
			}

			// Delete the SVI object from the DB
			err = infradb.client.Delete(svi.Name)
			if err != nil {
				log.Println(err)
				return err
			}

			// Delete the SVI from the svis map
			svis := make(map[string]bool)
			found, err = infradb.client.Get("svis", &svis)
			if err != nil {
				log.Println(err)
				return err
			}
			if !found {
				log.Println("UpdateSviStatus(): No Svis have been found")
				return ErrKeyNotFound
			}

			delete(svis, svi.Name)
			err = infradb.client.Set("svis", &svis)
			if err != nil {
				log.Println(err)
				return err
			}

			log.Printf("UpdateSviStatus(): Svi %s has been deleted\n", name)
		} else {
			svi.Status.SviOperStatus = SviOperStatusUp
			err = infradb.client.Set(svi.Name, svi)
			if err != nil {
				log.Println(err)
				return err
			}
			log.Printf("UpdateSviStatus(): SVI %s has been updated: %+v\n", name, svi)
		}
	} else {
		err = infradb.client.Set(svi.Name, svi)
		if err != nil {
			log.Println(err)
			return err
		}
		log.Printf("UpdateSviStatus(): SVI %s has been updated: %+v\n", name, svi)
	}

	taskmanager.TaskMan.StatusUpdated(svi.Name, "svi", svi.ResourceVersion, notificationID, false, &component)

	return nil
}

// SaveRoutingTable saves a routing table number to DB
func SaveRoutingTable(rtNum uint32) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	rts := make(map[uint32]bool)
	found, err := infradb.client.Get("rts", &rts)
	if err != nil {
		log.Println(err)
		return err
	}

	if !found {
		rts[rtNum] = false
		err = infradb.client.Set("rts", &rts)
		if err != nil {
			log.Println(err)
			return err
		}
		return nil
	}

	_, ok := rts[rtNum]
	if ok {
		log.Printf("SaveRoutingTable(): Routing Table %+v in use\n", rtNum)
		return ErrRoutingTableInUse
	}

	rts[rtNum] = false
	err = infradb.client.Set("rts", &rts)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

// DeleteRoutingTable deletes a routing table number from the DB
func DeleteRoutingTable(rtNum uint32) error {
	globalLock.Lock()
	defer globalLock.Unlock()

	rts := make(map[uint32]bool)
	found, err := infradb.client.Get("rts", &rts)
	if err != nil {
		log.Println(err)
		return err
	}

	if !found {
		log.Println("DeleteRoutingTable(): No routing tables have been found")
		return ErrKeyNotFound
	}

	_, ok := rts[rtNum]
	if !ok {
		log.Printf("DeleteRoutingTable(): Routing Table %+v not found\n", rtNum)
		return ErrKeyNotFound
	}

	delete(rts, rtNum)
	err = infradb.client.Set("rts", &rts)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}
