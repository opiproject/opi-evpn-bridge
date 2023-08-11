// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2023 Nordix Foundation.

package infradb

import (
	"encoding/json"
	"log"

	badger "github.com/dgraph-io/badger/v4"
	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"
)

var db *badger.DB

func Init() {

	var err error
	// Open the Badger database located in the /tmp/badger directory.
	// It will be created if it doesn't exist.
	db, err = badger.Open(badger.DefaultOptions("/tmp/badger"))
	if err != nil {
		log.Fatal(err)
	}
}

/*func Close() {
	// Close the DB
	db.Close()
}*/

// TransactionManager provides a wrapper for DB transactions
type TransactionManager interface {
	Add() error
	Update() error
	Read(resourceName string) error
	Delete() error
}

type VrfManager struct {
	VrfObj *pb.Vrf
}

func (v *VrfManager) Add() error {

	err := db.Update(func(txn *badger.Txn) error {
		value, err := json.Marshal(v.VrfObj)
		if err != nil {
			return err
		}
		return txn.Set([]byte(v.VrfObj.Name), value)
	})

	if err != nil {
		return err
	}

	return nil
}

func (v *VrfManager) Update() error {}

func (v *VrfManager) Read(resourceName string) error {
	value := []byte{}

	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(resourceName))
		if err != nil {
			return err
		}

		value, err = item.ValueCopy(nil)
		return nil
	})

	if err != nil {
		return err
	}

	err = json.Unmarshal(value, v.VrfObj)
	if err != nil {
		return err
	}
	return nil
}

func (v *VrfManager) Delete() error {}
