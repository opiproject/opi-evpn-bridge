// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package repository is the database abstraction implementing repository design pattern
package repository

import (
	"encoding/json"
)

// IVrfRepository abstraction
type IVrfRepository interface {
	SetVrf(resourceName string, vrf *Vrf) error
	GetVrf(resourceName string) (*Vrf, error)
	DeleteVrf(resourceName string) error
}

// Factory pattern to create new IVrfRepository
func VrfFactory(databaseImplementation string) (IVrfRepository, error) {
	kvstore, err := Factory(databaseImplementation)
	if err != nil {
		// TODO: use our own errors, maybe OperationError ?
		return nil, err
	}
	return newVrfDatabase(kvstore), nil
}

// sviDatabase implements IVrfRepository interface
type vrfDatabase struct {
	kvstore IKeyValueStore
}

func newVrfDatabase(kvstore IKeyValueStore) *vrfDatabase {
	return &vrfDatabase{
		kvstore: kvstore,
	}
}

func (repo *vrfDatabase) GetVrf(resourceName string) (*Vrf, error) {
	value, err := repo.kvstore.Get(resourceName)
	if err != nil {
		// TODO: use our own errors, maybe OperationError ?
		return nil, err
	}
	vrf := &Vrf{}
	err = json.Unmarshal([]byte(value), vrf)
	if err != nil {
		// TODO: use our own errors, maybe OperationError ?
		return nil, err
	}
	return vrf, nil
}

func (repo *vrfDatabase) SetVrf(resourceName string, vrf *Vrf) error {
	value, err := json.Marshal(vrf)
	if err != nil {
		return err
	}
	_, err = repo.kvstore.Set(resourceName, string(value))
	if err != nil {
		return err
	}
	return nil
}

func (repo *vrfDatabase) DeleteVrf(resourceName string) error {
	_, err := repo.kvstore.Delete(resourceName)
	if err != nil {
		return err
	}
	return nil
}
