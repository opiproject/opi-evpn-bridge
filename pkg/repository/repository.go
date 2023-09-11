// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package repository is the database abstraction implementing repository design pattern
package repository

// IKeyValueStore abstraction
type IKeyValueStore interface {
	Set(key string, value string) (string, error)
	Get(key string) (string, error)
	Delete(key string) (string, error)
}

// Factory pattern to create new IKeyValueStore
func Factory(databaseImplementation string) (IKeyValueStore, error) {
	switch databaseImplementation {
	case "redis":
		return newRedisDatabase()
	case "memory":
		return newMemoryDatabase(), nil
	default:
		return nil, &NotImplementedDatabaseError{databaseImplementation}
	}
}
