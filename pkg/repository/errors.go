// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package repository is the database abstraction implementing repository design pattern
package repository

// OperationError when cannot perform a given operation on database (SET, GET or DELETE)
type OperationError struct {
	operation string
}

func (err *OperationError) Error() string {
	return "Could not perform the " + err.operation + " operation."
}

// DownError when its not a redis.Nil response, in this case the database is down
type DownError struct{}

func (dbe *DownError) Error() string {
	return "Database is down"
}

// CreateDatabaseError when cannot perform set on database
type CreateDatabaseError struct{}

func (err *CreateDatabaseError) Error() string {
	return "Could not create Databse"
}

// NotImplementedDatabaseError when user tries to create a not implemented database
type NotImplementedDatabaseError struct {
	database string
}

func (err *NotImplementedDatabaseError) Error() string {
	return err.database + " not implemented"
}
