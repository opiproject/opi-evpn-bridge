// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package linuxgeneralmodule is the main package of the application

package utils

import (
	"log"
	"reflect"
)

// IDPool structure
/*  Helper class for uniquely assigning IDs from a specified integer set (e.g. a
#   range) to keys. IDs are assigned (or read) with get_id(key) and returned back
#   into the pool with release_id(key). The IDPool remembers a once-assigned ID
#   for keys so that the same ID is assigned for a key. Only when the pool runs
#   out of unassigned keys, it will recycle released ids and assign them to new
#   keys.
#   Optionally, the IDPool supports reference tracking for key/ID pairs. Clients
#   can provide a unique reference when fetching and releasing an ID for a key
#   to support multiple independent clients.
#   The pool will only release the ID for the key, when the last client has the
#   released the ID with its reference. When a reference is specified in get_id()
#   and release_id() the IDPool returns the current number of reference for the
#   ID so that a caller knows when an ID was newly assigned (ref_count 1) or
#   finally released (ref_count 0).
# ---------------------------------------------------------------------------------
*/
type IDPool struct {
	// self._lock = threading.Lock()
	name         string                 // Name of pool
	_unusedids   []uint32               // Yet unused IDs in pool Available ids
	_idsinuse    map[interface{}]uint32 // Mapping key: id for currently assigned ids
	_idsforreuse map[interface{}]uint32 // Mapping key: id for previously assigned ids
	_refs        map[uint32][]interface{}
	_size        int // Size of the pool
}

// IDPoolInit initialize mod ptr pool
func IDPoolInit(name string, min uint32, max uint32) IDPool {
	var id IDPool
	id.name = name
	id._unusedids = make([]uint32, 0)
	for j := min; j <= (max + 1); j++ {
		id._unusedids = append(id._unusedids, j)
	}
	id._size = len(id._unusedids)
	id._idsinuse = make(map[interface{}]uint32)
	id._idsforreuse = make(map[interface{}]uint32)
	id._refs = make(map[uint32][]interface{})
	return id
}

func (ip *IDPool) _assignID(key interface{}) uint32 {
	// Check if there was an id assigned for that key earlier
	id := ip._idsforreuse[key]
	if !reflect.ValueOf(id).IsZero() {
		// Re-use the old id
		delete(ip._idsforreuse, key)
	} else {
		if len(ip._unusedids) != 0 {
			// Pick an unused id
			id = ip._unusedids[0]
			ip._unusedids = append(ip._unusedids[:0], ip._unusedids[1:]...)
		} else {
			if len(ip._idsforreuse) != 0 {
				// Pick one of the ids earlier used for another key
				for oldKey := range ip._idsforreuse {
					delete(ip._idsforreuse, oldKey)
					break
				}
			} else {
				// No id left
				log.Printf("IDPool: Failed to allocate id for %+v. No free ids in pool.", key)
				return 0
			}
		}
	}
	// Store the assigned id, if any
	if !reflect.ValueOf(id).IsZero() {
		ip._idsinuse[key] = id
	}
	return id
}

// GetID get the mod ptr id from pool
func (ip *IDPool) GetID(key interface{}, ref interface{}) (uint32, uint32) {
	id := ip._idsinuse[key]
	if reflect.ValueOf(id).IsZero() {
		// Assign a free id for the key
		id = ip._assignID(key)
		if id == 0 {
			return 0, 0
		}
	}
	if !reflect.ValueOf(ref).IsZero() {
		log.Printf("IDPool: GetID  Assigning key : %+v , id  %+v for ref %v", id, key, ref)
		// refSet := ip._refs[id]
		if reflect.ValueOf(ip._refs[id]).IsZero() {
			ip._refs[id] = make([]interface{}, 0)
		}
		ip._refs[id] = append(ip._refs[id], ref)
		return id, uint32(len(ip._refs[id]))
	}
	log.Printf("IDPool: GetID Assigning id %v for key %v and ref %v", id, key, ref)
	return id, uint32(0)
}

func deleteRef(refSet []interface{}, ref interface{}) []interface{} {
	// size := len(refSet)
	var i uint32
	for index, value := range refSet {
		if value == ref {
			i = uint32(index)
			break
		}
	}
	return append(refSet[:i], refSet[i+1:]...)
}

// ReleaseID get the reference id
func (ip *IDPool) ReleaseID(key interface{}, ref interface{}) (uint32, uint32) {
	// with self._lock:
	log.Printf("IDPool:ReleaseID  Releasing id for key %v", key)
	id := ip._idsinuse[key]
	if reflect.ValueOf(ref).IsZero() {
		log.Printf("No id to release for key %v", key)
		return 0, 0
	}
	refSet := ip._refs[id]
	if !reflect.ValueOf(refSet).IsZero() && !reflect.ValueOf(ref).IsZero() {
		// Remove the specified reference from the id
		refSet = deleteRef(refSet, ref)
	}
	if !reflect.ValueOf(refSet).IsZero() {
		// No (remaining) references, release id
		log.Printf("IDPool:ReleaseID Id %v has been released", id)
		delete(ip._idsinuse, key)
		if !reflect.ValueOf(refSet).IsZero() {
			delete(ip._refs, id)
		}
		// Store released id for future reassignment
		ip._idsforreuse[key] = id
	} else {
		log.Printf("IDPool:ReleaseID Keep id:%+v remaining references %+v", id, len(refSet))
	}
	if !reflect.ValueOf(ref).IsZero() {
		return id, uint32(len(refSet))
	}
	return id, uint32(0)
}
