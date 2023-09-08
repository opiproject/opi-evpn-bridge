// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package repository is the database abstraction implementing repository design pattern
package repository

import "github.com/go-redis/redis"

type redisDatabase struct {
	client *redis.Client
}

func newRedisDatabase() (*redisDatabase, error) {
	// TODO: pass address
	url := "redis://redis:6379?password=&protocol=3/vrf"
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, &CreateDatabaseError{}
	}
	client := redis.NewClient(opts)
	_, err = client.Ping().Result() // makes sure database is connected
	if err != nil {
		return nil, &CreateDatabaseError{}
	}
	return &redisDatabase{client: client}, nil
}

func (r *redisDatabase) Set(key string, value string) (string, error) {
	_, err := r.client.Set(key, value, 0).Result()
	if err != nil {
		return generateError("set", err)
	}
	return key, nil
}

func (r *redisDatabase) Get(key string) (string, error) {
	value, err := r.client.Get(key).Result()
	if err != nil {
		return generateError("get", err)
	}
	return value, nil
}

func (r *redisDatabase) Delete(key string) (string, error) {
	_, err := r.client.Del(key).Result()
	if err != nil {
		return generateError("delete", err)
	}
	return key, nil
}

func generateError(operation string, err error) (string, error) {
	if err == redis.Nil {
		return "", &OperationError{operation}
	}
	return "", &DownError{}
}
