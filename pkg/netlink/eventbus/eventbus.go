// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.

// Package eventbus handles pub sub
package eventbus

import (
	"sync"
)

// EventBus holds the event bus info
type EventBus struct {
	subscribers map[string][]*Subscriber
	mutex       sync.Mutex
}

// Subscriber holds the info for each subscriber
type Subscriber struct {
	Ch   chan interface{}
	Quit chan struct{}
}

// NewEventBus initializes an EventBus object
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]*Subscriber),
	}
}

// Subscribe api provides registration of a subscriber to the given eventType
func (e *EventBus) Subscribe(eventType string) *Subscriber {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	subscriber := &Subscriber{
		Ch:   make(chan interface{}),
		Quit: make(chan struct{}),
	}

	e.subscribers[eventType] = append(e.subscribers[eventType], subscriber)

	return subscriber
}

// Publish api notifies the subscribers with certain eventType
func (e *EventBus) Publish(eventType string, data interface{}) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	subscribers, ok := e.subscribers[eventType]
	if !ok {
		return
	}

	for _, sub := range subscribers {
		sub.Ch <- data
	}
}

// Unsubscribe closes all subscriber channels and empties the subscriber map.
func (e *EventBus) Unsubscribe() {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for eventName, subs := range e.subscribers {
		for _, sub := range subs {
			close(sub.Ch) // Close each channel
		}
		delete(e.subscribers, eventName) // Remove the entry from the map
	}
}
