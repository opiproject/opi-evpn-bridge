// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package linuxgeneralmodule is the main package of the application

package utils

import (
	"log"
	"reflect"
	//"github.com/opiproject/opi-evpn-bridge/pkg/vendor_plugins/intel-e2000/p4runtime/p4translation"
)

/*
# ---------------------------------------------------------------------------------
#   ID Pool
#
#   Helper class for uniquely assigning IDs from a specified integer set (e.g. a
#   range) to keys. IDs are assigned (or read) with get_id(key) and returned back
#   into the pool with release_id(key). The IdPool remembers a once-assigned ID
#   for keys so that the same ID is assigned for a key. Only when the pool runs
#   out of unassigned keys, it will recycle released ids and assign them to new
#   keys.
#   Optionally, the IdPool supports reference tracking for key/ID pairs. Clients
#   can provide a unique reference when fetching and releasing an ID for a key
#   to support multiple independent clients.
#   The pool will only release the ID for the key, when the last client has the
#   released the ID with its reference. When a reference is specified in get_id()
#   and release_id() the IdPool returns the current number of reference for the
#   ID so that a caller knows when an ID was newly assigned (ref_count 1) or
#   finally released (ref_count 0).
# ---------------------------------------------------------------------------------
*/

type IdPool struct {
	//self._lock = threading.Lock()
	name           string                 // Name of pool
	_unused_ids    []uint32               // Yet unused IDs in pool Available ids
	_ids_in_use    map[interface{}]uint32 // Mapping key: id for currently assigned ids
	_ids_for_reuse map[interface{}]uint32 // Mapping key: id for previously assigned ids
	_refs          map[uint32][]interface{}
	_size          int // Size of the pool
}

// IDPoolInit initialize mod ptr pool
func IDPoolInit(name string, min uint32, max uint32) IdPool {
	var id IdPool
	id.name = name
	id._unused_ids = make([]uint32, 0)
	for j := min; j <= (max + 1); j++ {
		id._unused_ids = append(id._unused_ids, j)
	}
	id._size = len(id._unused_ids)
	id._ids_in_use = make(map[interface{}]uint32)
	id._ids_for_reuse = make(map[interface{}]uint32)
	id._refs = make(map[uint32][]interface{})
	return id
}

func (Ip *IdPool) _assign_id(key interface{}) uint32 {
	// Check if there was an id assigned for that key earlier
	id := Ip._ids_for_reuse[key]
	if !reflect.ValueOf(id).IsZero() {
		// Re-use the old id
		delete(Ip._ids_for_reuse, key)
	} else {
		if len(Ip._unused_ids) != 0 {
			// Pick an unused id
			id = Ip._unused_ids[0]
			Ip._unused_ids = append(Ip._unused_ids[1:])
		} else {
			if len(Ip._ids_for_reuse) != 0 {
				// Pick one of the ids earlier used for another key
				for old_key, _ := range Ip._ids_for_reuse {
					delete(Ip._ids_for_reuse, old_key)
					break
				}
			} else {
				// No id left
				log.Printf("Failed to allocate id for %+v. No free ids in pool.", key)
			}
		}
	}
	// Store the assigned id, if any
	if !reflect.ValueOf(id).IsZero() {
		Ip._ids_in_use[key] = id
	}
	return id
}

// getID get the mod ptr id from pool
func (IP *IdPool) GetID(key interface{}, ref interface{}) (uint32, uint32) {
	id := IP._ids_in_use[key]
	if reflect.ValueOf(id).IsZero() {
		// Assign a free id for the key
		id = IP._assign_id(key)
	}
	if reflect.ValueOf(ref).IsZero() {
		log.Printf("assigning id for ref %v", ref)
		ref_set := IP._refs[id]
		if reflect.ValueOf(ref_set).IsZero() {
			ref_set = append(ref_set, ref)
			IP._refs[id] = ref_set
			return id, uint32(len(ref_set))
		}
	} else {
		log.Printf("assigning id %v for key %v and ref %v", id, key, ref)
	}
	return id, uint32(0)
}

func delete_ref(ref_set []interface{}, ref interface{}) []interface{} {
	//size := len(ref_set)
	var i uint32
	for index, value := range ref_set {
		if value == ref {
			i = uint32(index)
			break
		}
	}
	return append(ref_set[:i], ref_set[i+1:]...)
}

// refCount get the reference count
func (IP *IdPool) Release_id(key interface{}, ref interface{}) (uint32, uint32) {
	//with self._lock:
	log.Printf("releasing id for key %v", key)
	id := IP._ids_in_use[key]
	if reflect.ValueOf(ref).IsZero() {
		log.Printf("No id to release for key %v", key)
		return 0, 0
	}
	ref_set := IP._refs[id]
	if !reflect.ValueOf(ref_set).IsZero() && !reflect.ValueOf(ref).IsZero() {
		// Remove the specified reference from the id
		ref_set = delete_ref(ref_set, ref)
	}
	if !reflect.ValueOf(ref_set).IsZero() {
		// No (remaining) references, release id
		log.Printf("id %v has been released", id)
		delete(IP._ids_in_use, key)
		if !reflect.ValueOf(ref_set).IsZero() {
			delete(IP._refs, id)
		}
		// Store released id for future reassignment
		IP._ids_for_reuse[key] = id
	} else {
		log.Printf("Keep id. %v remaining references", len(ref_set))
	}
	if !reflect.ValueOf(ref).IsZero() {
		return id, uint32(len(ref_set))
	} else {
		return id, uint32(0)
	}
}
