// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2023 Nordix Foundation.

package infradb

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	badger "github.com/dgraph-io/badger/v4"
)

const (
	// discardRatio. It represents the discard ratio for the GC.
	//
	// Ref: https://godoc.org/github.com/dgraph-io/badger#DB.RunValueLogGC
	discardRatio = 0.5

	// GC interval
	gcInterval = 10 * time.Minute
)

var idb *InfraDB

type InfraDB struct {
	db         *badger.DB
	ctx        context.Context
	cancelFunc context.CancelFunc
}

func NewInfraDB(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0774); err != nil {
		return err
	}

	opts := badger.DefaultOptions(dataDir)

	badgerDB, err := badger.Open(opts)
	if err != nil {
		return err
	}

	idb = &InfraDB{
		db: badgerDB,
	}
	idb.ctx, idb.cancelFunc = context.WithCancel(context.Background())

	go idb.execGC()
	return nil
}

func Close() error {
	return idb.close()
}

func GetVrf(resourceName string) (*Vrf, error) {
	return idb.getVrf(resourceName)
}

func AddVrf(Vrf *Vrf) error {
	return idb.addVrf(Vrf)
}

func DeleteVrf(resourceName string) error {
	return idb.deleteVrf(resourceName)
}

func (d *InfraDB) close() error {
	d.cancelFunc()
	return d.db.Close()
}

func (d *InfraDB) execGC() {
	ticker := time.NewTicker(gcInterval)
	for {
		select {
		case <-ticker.C:
			err := d.db.RunValueLogGC(discardRatio)
			if err != nil {
				// don't report error when GC didn't result in any cleanup
				if err == badger.ErrNoRewrite {
					log.Printf("no BadgerDB GC occurred: %v", err)
				} else {
					log.Printf("failed to GC BadgerDB: %v", err)
				}
			}

		case <-d.ctx.Done():
			return
		}
	}
}

func (d *InfraDB) getVrf(resourceName string) (*Vrf, error) {
	vrf := &Vrf{}
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
		return nil, err
	}

	err = json.Unmarshal(value, vrf)
	if err != nil {
		return nil, err
	}
	return vrf, nil
}

func (d *InfraDB) addVrf(Vrf *Vrf) error {
	err := db.Update(func(txn *badger.Txn) error {
		value, err := json.Marshal(Vrf)
		if err != nil {
			return err
		}
		return txn.Set([]byte(Vrf.PbVrf.Name), value)
	})

	if err != nil {
		return err
	}

	return nil
}

func (d *InfraDB) deleteVrf(resourceName string) error {
	err := db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(resourceName))
	})

	if err != nil {
		return err
	}

	return nil
}
