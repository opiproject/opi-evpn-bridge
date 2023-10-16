// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Code generated by mockery v2.35.4. DO NOT EDIT.

package mocks

import (
	context "context"

	mock "github.com/stretchr/testify/mock"
	telnet "github.com/ziutek/telnet"
)

// Frr is an autogenerated mock type for the Frr type
type Frr struct {
	mock.Mock
}

type Frr_Expecter struct {
	mock *mock.Mock
}

func (_m *Frr) EXPECT() *Frr_Expecter {
	return &Frr_Expecter{mock: &_m.Mock}
}

// FrrBgpCmd provides a mock function with given fields: ctx, command
func (_m *Frr) FrrBgpCmd(ctx context.Context, command string) (string, error) {
	ret := _m.Called(ctx, command)

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (string, error)); ok {
		return rf(ctx, command)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) string); ok {
		r0 = rf(ctx, command)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, command)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Frr_FrrBgpCmd_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'FrrBgpCmd'
type Frr_FrrBgpCmd_Call struct {
	*mock.Call
}

// FrrBgpCmd is a helper method to define mock.On call
//   - ctx context.Context
//   - command string
func (_e *Frr_Expecter) FrrBgpCmd(ctx interface{}, command interface{}) *Frr_FrrBgpCmd_Call {
	return &Frr_FrrBgpCmd_Call{Call: _e.mock.On("FrrBgpCmd", ctx, command)}
}

func (_c *Frr_FrrBgpCmd_Call) Run(run func(ctx context.Context, command string)) *Frr_FrrBgpCmd_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string))
	})
	return _c
}

func (_c *Frr_FrrBgpCmd_Call) Return(_a0 string, _a1 error) *Frr_FrrBgpCmd_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *Frr_FrrBgpCmd_Call) RunAndReturn(run func(context.Context, string) (string, error)) *Frr_FrrBgpCmd_Call {
	_c.Call.Return(run)
	return _c
}

// FrrZebraCmd provides a mock function with given fields: ctx, command
func (_m *Frr) FrrZebraCmd(ctx context.Context, command string) (string, error) {
	ret := _m.Called(ctx, command)

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string) (string, error)); ok {
		return rf(ctx, command)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string) string); ok {
		r0 = rf(ctx, command)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, command)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Frr_FrrZebraCmd_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'FrrZebraCmd'
type Frr_FrrZebraCmd_Call struct {
	*mock.Call
}

// FrrZebraCmd is a helper method to define mock.On call
//   - ctx context.Context
//   - command string
func (_e *Frr_Expecter) FrrZebraCmd(ctx interface{}, command interface{}) *Frr_FrrZebraCmd_Call {
	return &Frr_FrrZebraCmd_Call{Call: _e.mock.On("FrrZebraCmd", ctx, command)}
}

func (_c *Frr_FrrZebraCmd_Call) Run(run func(ctx context.Context, command string)) *Frr_FrrZebraCmd_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string))
	})
	return _c
}

func (_c *Frr_FrrZebraCmd_Call) Return(_a0 string, _a1 error) *Frr_FrrZebraCmd_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *Frr_FrrZebraCmd_Call) RunAndReturn(run func(context.Context, string) (string, error)) *Frr_FrrZebraCmd_Call {
	_c.Call.Return(run)
	return _c
}

// Password provides a mock function with given fields: conn, delim
func (_m *Frr) Password(conn *telnet.Conn, delim string) error {
	ret := _m.Called(conn, delim)

	var r0 error
	if rf, ok := ret.Get(0).(func(*telnet.Conn, string) error); ok {
		r0 = rf(conn, delim)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Frr_Password_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Password'
type Frr_Password_Call struct {
	*mock.Call
}

// Password is a helper method to define mock.On call
//   - conn *telnet.Conn
//   - delim string
func (_e *Frr_Expecter) Password(conn interface{}, delim interface{}) *Frr_Password_Call {
	return &Frr_Password_Call{Call: _e.mock.On("Password", conn, delim)}
}

func (_c *Frr_Password_Call) Run(run func(conn *telnet.Conn, delim string)) *Frr_Password_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*telnet.Conn), args[1].(string))
	})
	return _c
}

func (_c *Frr_Password_Call) Return(_a0 error) *Frr_Password_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Frr_Password_Call) RunAndReturn(run func(*telnet.Conn, string) error) *Frr_Password_Call {
	_c.Call.Return(run)
	return _c
}

// Privileged provides a mock function with given fields: conn
func (_m *Frr) Privileged(conn *telnet.Conn) error {
	ret := _m.Called(conn)

	var r0 error
	if rf, ok := ret.Get(0).(func(*telnet.Conn) error); ok {
		r0 = rf(conn)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// Frr_Privileged_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'Privileged'
type Frr_Privileged_Call struct {
	*mock.Call
}

// Privileged is a helper method to define mock.On call
//   - conn *telnet.Conn
func (_e *Frr_Expecter) Privileged(conn interface{}) *Frr_Privileged_Call {
	return &Frr_Privileged_Call{Call: _e.mock.On("Privileged", conn)}
}

func (_c *Frr_Privileged_Call) Run(run func(conn *telnet.Conn)) *Frr_Privileged_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*telnet.Conn))
	})
	return _c
}

func (_c *Frr_Privileged_Call) Return(_a0 error) *Frr_Privileged_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Frr_Privileged_Call) RunAndReturn(run func(*telnet.Conn) error) *Frr_Privileged_Call {
	_c.Call.Return(run)
	return _c
}

// TelnetDialAndCommunicate provides a mock function with given fields: ctx, command, port
func (_m *Frr) TelnetDialAndCommunicate(ctx context.Context, command string, port int) (string, error) {
	ret := _m.Called(ctx, command, port)

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(context.Context, string, int) (string, error)); ok {
		return rf(ctx, command, port)
	}
	if rf, ok := ret.Get(0).(func(context.Context, string, int) string); ok {
		r0 = rf(ctx, command, port)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(context.Context, string, int) error); ok {
		r1 = rf(ctx, command, port)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Frr_TelnetDialAndCommunicate_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'TelnetDialAndCommunicate'
type Frr_TelnetDialAndCommunicate_Call struct {
	*mock.Call
}

// TelnetDialAndCommunicate is a helper method to define mock.On call
//   - ctx context.Context
//   - command string
//   - port int
func (_e *Frr_Expecter) TelnetDialAndCommunicate(ctx interface{}, command interface{}, port interface{}) *Frr_TelnetDialAndCommunicate_Call {
	return &Frr_TelnetDialAndCommunicate_Call{Call: _e.mock.On("TelnetDialAndCommunicate", ctx, command, port)}
}

func (_c *Frr_TelnetDialAndCommunicate_Call) Run(run func(ctx context.Context, command string, port int)) *Frr_TelnetDialAndCommunicate_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(context.Context), args[1].(string), args[2].(int))
	})
	return _c
}

func (_c *Frr_TelnetDialAndCommunicate_Call) Return(_a0 string, _a1 error) *Frr_TelnetDialAndCommunicate_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *Frr_TelnetDialAndCommunicate_Call) RunAndReturn(run func(context.Context, string, int) (string, error)) *Frr_TelnetDialAndCommunicate_Call {
	_c.Call.Return(run)
	return _c
}

// NewFrr creates a new instance of Frr. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewFrr(t interface {
	mock.TestingT
	Cleanup(func())
}) *Frr {
	mock := &Frr{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
