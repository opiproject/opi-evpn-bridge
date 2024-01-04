// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package storage for string the db
package storage

import (
	"fmt"
	"log"

	"github.com/philippgille/gokv"
	"github.com/philippgille/gokv/gomap"
	"github.com/philippgille/gokv/redis"
)

var st *Storage

// Storage is an implementation of KeyValueStore using the gokv library.
type Storage struct {
	store gokv.Store
}

// NewStore creates a new Storage instance based on the specified backend.
// Supported backends: "redis".
func NewStore(backend, address string) (*Storage, error) {
	var store gokv.Store
	var err error

	switch backend {
	case "redis":

		options := redis.DefaultOptions
		options.Address = address
		store, err = redis.NewClient(options)
	case "gomap":

		options := gomap.DefaultOptions
		store = gomap.NewStore(options)
	default:
		return nil, fmt.Errorf("unsupported backend: %s", backend)
	}

	if err != nil {
		return nil, err
	}
	st = &Storage{store: store}
	return st, nil
}

// GetStore returns the underlying database client.
func GetStore() gokv.Store {
	if st.store == nil {
		log.Fatal(" Store not present ")
	}
	return st.store
}

// GetClient returns the underlying database client.
func (s *Storage) GetClient() gokv.Store {
	return s.store
}

// Set stores the key-value pair in the store.
func (s *Storage) Set(key string, value interface{}) error {
	return s.store.Set(key, value)
}

// Get retrieves the value associated with the given key.
func (s *Storage) Get(key string, value interface{}) (bool, error) {
	found, err := s.store.Get(key, value)
	if err != nil {
		return found, err
	}
	return found, err
}

// Delete removes the key-value pair from the store.
func (s *Storage) Delete(key string) error {
	return s.store.Delete(key)
}

// Close releases any resources held by the store.
func (s *Storage) Close() error {
	return s.store.Close()
}
