// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package repository is the database abstraction implementing repository design pattern
package repository

import (
	"encoding/json"
)

// ISviRepository abstraction
type ISviRepository interface {
	SetSvi(resourceName string, svi *Svi) error
	GetSvi(resourceName string) (*Svi, error)
	DeleteSvi(resourceName string) error
}

// Factory pattern to create new ISviRepository
func SviFactory(databaseImplementation string) (ISviRepository, error) {
	kvstore, err := Factory(databaseImplementation)
	if err != nil {
		// TODO: use our own errors, maybe OperationError ?
		return nil, err
	}
	return newSviDatabase(kvstore), nil
}

// sviDatabase implements ISviRepository interface
type sviDatabase struct {
	kvstore IKeyValueStore
}

func newSviDatabase(kvstore IKeyValueStore) *sviDatabase {
	return &sviDatabase{
		kvstore: kvstore,
	}
}

func (repo *sviDatabase) GetSvi(resourceName string) (*Svi, error) {
	value, err := repo.kvstore.Get(resourceName)
	if err != nil {
		// TODO: use our own errors, maybe OperationError ?
		return nil, err
	}
	svi := &Svi{}
	err = json.Unmarshal([]byte(value), svi)
	if err != nil {
		// TODO: use our own errors, maybe OperationError ?
		return nil, err
	}
	return svi, nil
}

func (repo *sviDatabase) SetSvi(resourceName string, svi *Svi) error {
	value, err := json.Marshal(svi)
	if err != nil {
		return err
	}
	_, err = repo.kvstore.Set(resourceName, string(value))
	if err != nil {
		return err
	}
	return nil
}

func (repo *sviDatabase) DeleteSvi(resourceName string) error {
	_, err := repo.kvstore.Delete(resourceName)
	if err != nil {
		return err
	}
	return nil
}
