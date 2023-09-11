// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package repository is the database abstraction implementing repository design pattern
package repository

import (
	"fmt"
	"sync"
)

type memoryDatabase struct {
	data map[string]string
	lock sync.RWMutex
}

func newMemoryDatabase() *memoryDatabase {
	return &memoryDatabase{
		data: make(map[string]string),
		// lock: &sync.RWMutex{},
	}
}

func (repo *memoryDatabase) Set(key string, value string) (string, error) {
	repo.lock.RLock()
	defer repo.lock.RUnlock()
	repo.data[key] = value
	return key, nil
}

func (repo *memoryDatabase) Get(key string) (string, error) {
	repo.lock.RLock()
	defer repo.lock.RUnlock()
	value, ok := repo.data[key]
	if !ok {
		// TODO: use our own errors, maybe OperationError ?
		return "", fmt.Errorf("value does not exist for key: %s", key)
	}
	return value, nil
}

func (repo *memoryDatabase) Delete(key string) (string, error) {
	repo.lock.RLock()
	defer repo.lock.RUnlock()
	delete(repo.data, key)
	return key, nil
}
