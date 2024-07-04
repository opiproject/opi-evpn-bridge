package netlink

import (
	"log"
	"reflect"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
)

// notifyDBChanges notify the database changes
func notifyDBChanges() {
	process[routeKey, *RouteStruct](latestRoutes, routes, ROUTE, RouteOperations)
	process[nexthopKey, *NexthopStruct](latestNexthop, Nexthops, NEXTHOP, NexthopOperations)
	process[fdbKey, *FdbEntryStruct](latestFDB, fDB, FDB, FdbOperations)
	process[l2NexthopKey, *L2NexthopStruct](latestL2Nexthop, l2Nexthops, L2NEXTHOP, L2NexthopOperations)
}

func process[K comparable, V any](latestDB, oldDB map[K]V, eventType int, ops Operations) {
	oldgenmap = make(map[interface{}]interface{})
	latestgenmap = make(map[interface{}]interface{})
	for k, v := range latestDB {
		latestgenmap[k] = v
	}
	for k, v := range oldDB {
		oldgenmap[k] = v
	}
	event := Event{
		EventType: eventType,
		Operation:/*Operate{
			toAdd:    ops.Add,
			toUpdate: ops.Update,
			toDelete: ops.Delete,
		}*/ops,
	}
	notify_changes(latestgenmap, oldgenmap, event)
}

// nolint
func notify_changes(new_db map[interface{}]interface{}, old_db map[interface{}]interface{}, event Event) {
	DB2 := old_db
	DB1 := new_db
	/* Checking the Updated entries in the netlink db by comparing the individual keys and their corresponding values in old and new db copies
	   entries with same keys with different values and send the notification to vendor specific module */
	for k1, v1 := range DB1 {
		v2, ok := DB2[k1]
		if !ok {
			continue
		}
		if !deepCheck(v1, v2) {
			// To Avoid in-correct update notification due to race condition in which metadata is nil in new entry and crashing in dcgw module
			if event.EventType == ROUTE || event.EventType == NEXTHOP {
				var status VrfStatusGetter
				var ok bool
				status, ok = v1.(VrfStatusGetter)
				if !ok {
					log.Printf("Netlink: Invalid Type")
					continue
				}
				if status.GetVrfOperStatus() == infradb.VrfOperStatusToBeDeleted {
					notifyAddDel(status, event.Operation.Delete)
					delete(new_db, k1)
					delete(old_db, k1)
					continue
				}
			}
			notifyAddDel(v1, event.Operation.Update)
		}
		delete(new_db, k1)
		delete(old_db, k1)
	}
	for _, r := range new_db { // Added entries notification cases
		notifyAddDel(r, event.Operation.Add)
	}
	for _, r := range old_db { // Deleted entires notification cases
		notifyAddDel(r, event.Operation.Delete)
	}
}

func deepCheck(v1 interface{}, v2 interface{}) bool {
	if reflect.TypeOf(v1) != reflect.TypeOf(v2) {
		return true
	}
	switch t := v1.(type) {
	case *RouteStruct:
		return t.deepEqual(v2.(*RouteStruct), true)
	case *NexthopStruct:
		return t.deepEqual(v2.(*NexthopStruct), true)
	case *FdbEntryStruct:
		return t.deepEqual(v2.(*FdbEntryStruct), true)
	case *L2NexthopStruct:
		return t.deepEqual(v2.(*L2NexthopStruct), true)
	default:
		return true
	}
}

// GetVrfOperStatus gets route vrf operation status
func (route *RouteStruct) GetVrfOperStatus() infradb.VrfOperStatus {
	return route.Vrf.Status.VrfOperStatus
}

// GetVrfOperStatus gets nexthop vrf opration status
func (nexthop *NexthopStruct) GetVrfOperStatus() infradb.VrfOperStatus {
	return nexthop.Vrf.Status.VrfOperStatus
}

func notifyAddDel(r interface{}, event string) {
	log.Printf("netlink: Notify event: %s\n", event)
	EventBus.Publish(event, r)
}

func (l2n *L2NexthopStruct) deepEqual(l2nOld *L2NexthopStruct, nc bool) bool {
	if l2n.Dev != l2nOld.Dev || !l2n.Dst.Equal(l2nOld.Dst) || l2n.ID != l2nOld.ID || l2n.Key != l2nOld.Key || l2n.Type != l2nOld.Type {
		return false
	}
	if nc {
		if len(l2n.FdbRefs) != len(l2nOld.FdbRefs) {
			return false
		}
		for i := range l2n.FdbRefs {
			ret := l2n.FdbRefs[i].deepEqual(l2nOld.FdbRefs[i], false)
			if !ret {
				return false
			}
		}
	}
	return true
}

func (fdb *FdbEntryStruct) deepEqual(fdbOld *FdbEntryStruct, nc bool) bool {
	if fdb.VlanID != fdbOld.VlanID || fdb.Mac != fdbOld.Mac || fdb.Key != fdbOld.Key || fdb.Type != fdbOld.Type {
		return false
	}
	if nc {
		if fdb.Nexthop != nil || fdbOld != nil {
			ret := fdb.Nexthop.deepEqual(fdbOld.Nexthop, false)
			if !ret {
				return false
			}
		}
	}
	return true
}

func (nexthop *NexthopStruct) deepEqual(nhOld *NexthopStruct, nc bool) bool {
	if nexthop.Vrf.Name != nhOld.Vrf.Name || nexthop.Weight != nhOld.Weight || nexthop.ID != nhOld.ID || nexthop.Key != nhOld.Key || nexthop.Local != nhOld.Local ||
		!reflect.DeepEqual(nexthop.Metadata, nhOld.Metadata) || nexthop.Metric != nhOld.Metric ||
		nexthop.Scope != nhOld.Scope || nexthop.Resolved != nhOld.Resolved || nexthop.Protocol != nhOld.Protocol || nexthop.NhType != nhOld.NhType ||
		!reflect.DeepEqual(nexthop.nexthop, nhOld.nexthop) {
		return false
	}
	if nc {
		if len(nexthop.RouteRefs) != len(nhOld.RouteRefs) {
			return false
		}
		for i := range nexthop.RouteRefs {
			ret := nexthop.RouteRefs[i].deepEqual(nhOld.RouteRefs[i], false)
			if !ret {
				return false
			}
		}
	}
	return true
}

func (route *RouteStruct) deepEqual(routeOld *RouteStruct, nc bool) bool {
	if route.Vrf.Name != routeOld.Vrf.Name || !reflect.DeepEqual(route.Route0, routeOld.Route0) || route.Key != routeOld.Key ||
		route.NlType != routeOld.NlType || !reflect.DeepEqual(route.Metadata, routeOld.Metadata) {
		return false
	}
	if nc {
		if len(route.Nexthops) != len(routeOld.Nexthops) {
			return false
		}
		return route.Nexthops[0].deepEqual(routeOld.Nexthops[0], false)
	}
	return true
}
