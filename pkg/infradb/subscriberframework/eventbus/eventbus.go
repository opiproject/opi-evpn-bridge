// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package eventbus holds implementation for subscribing and receiving events
package eventbus

import (
	"fmt"
	"log"
	"sort"
	"sync"
)

// EBus holds the EventBus object
var EBus = NewEventBus()

// EventBus holds the event bus info
type EventBus struct {
	subscribers   map[string][]*Subscriber
	eventHandlers map[string]EventHandler
	subscriberL   sync.RWMutex
	publishL      sync.RWMutex
	mutex         sync.RWMutex
}

// Subscriber holds the info for each subscriber
type Subscriber struct {
	Name     string
	Ch       chan interface{}
	Quit     chan bool
	Priority int
}

// EventHandler handles the events that arrive
type EventHandler interface {
	HandleEvent(string, *ObjectData)
}

// ObjectData holds data related to the objects to be realized
type ObjectData struct {
	ResourceVersion string
	Name            string
	NotificationID  string
}

// StartSubscriber will be called by the modules to initialize and start listening for events
func (e *EventBus) StartSubscriber(moduleName, eventType string, priority int, eventHandler EventHandler) {
	if !e.subscriberExist(eventType, moduleName) {
		subscriber := e.Subscribe(moduleName, eventType, priority, eventHandler)

		go func() {
			for {
				select {
				case event := <-subscriber.Ch:
					log.Printf("\nSubscriber %s for %s received \n", moduleName, eventType)

					handlerKey := moduleName + "." + eventType
					if handler, ok := e.eventHandlers[handlerKey]; ok {
						if objectData, ok := event.(*ObjectData); ok {
							handler.HandleEvent(eventType, objectData)
						} else {
							subscriber.Ch <- "error: unexpected event type"
						}
						// handler.HandleEvent(eventType, event)
					} else {
						subscriber.Ch <- "error: no event handler found"
					}
				case <-subscriber.Quit:
					close(subscriber.Ch)
					return
				}
			}
		}()
	}
}

// NewEventBus initializes ann EventBus object
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers:   make(map[string][]*Subscriber),
		eventHandlers: make(map[string]EventHandler),
	}
}

// Subscribe api provides registration of a subscriber to the given eventType
func (e *EventBus) Subscribe(moduleName, eventType string, priority int, eventHandler EventHandler) *Subscriber {
	e.subscriberL.Lock()
	defer e.subscriberL.Unlock()

	subscriber := &Subscriber{
		Name:     moduleName,
		Ch:       make(chan interface{}),
		Quit:     make(chan bool),
		Priority: priority,
	}

	e.subscribers[eventType] = append(e.subscribers[eventType], subscriber)
	e.eventHandlers[moduleName+"."+eventType] = eventHandler

	// Sort subscribers based on priority
	sort.Slice(e.subscribers[eventType], func(i, j int) bool {
		return e.subscribers[eventType][i].Priority < e.subscribers[eventType][j].Priority
	})

	log.Printf("Subscriber %s registered for event %s with priority %d\n", moduleName, eventType, priority)
	return subscriber
}

// GetSubscribers api is used to fetch the list of subscribers registered with given eventType is priority order
// first in list has the higher priority followed by others and so on
func (e *EventBus) GetSubscribers(eventType string) []*Subscriber {
	e.mutex.RLock()
	defer e.mutex.RUnlock()

	return e.subscribers[eventType]
}

func (e *EventBus) subscriberExist(eventType string, moduleName string) bool {
	subList := e.GetSubscribers(eventType)
	if len(subList) != 0 {
		for _, s := range subList {
			if s.Name == moduleName {
				return true
			}
		}
	}
	return false
}

// Publish api notifies the subscribers with certain eventType
func (e *EventBus) Publish(objectData *ObjectData, subscriber *Subscriber) error {
	e.publishL.RLock()
	defer e.publishL.RUnlock()
	var err error
	// We need the default case here as if the subscriber is busy then we will not be able to sent to the subscriber channel
	// and the Publish function will stuck. So the default case serves exctly this purpose.
	select {
	case subscriber.Ch <- objectData:
		log.Printf("Publish(): Notification is sent to subscriber %s\n", subscriber.Name)
	default:
		log.Printf("Publish(): Channel for subsriber %s is busy. Notification not sent", subscriber.Name)
		err = fmt.Errorf("channel is busy")
	}
	return err
}

// Unsubscribe the subscriber, which delete the subscriber(all resources will be washed out)
func (e *EventBus) Unsubscribe(subscriber *Subscriber) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	subscriber.Quit <- true
	log.Printf("\nSubscriber %s is unsubscribed for all events\n", subscriber.Name)
}

// Unsubscribe closes the event channel
func (s *Subscriber) Unsubscribe() {
	close(s.Ch)
}

// UnsubscribeEvent will unsubscribe particular eventType of a subscriber
func (e *EventBus) UnsubscribeEvent(subscriber *Subscriber, eventType string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if subscribers, ok := e.subscribers[eventType]; ok {
		for i, sub := range subscribers {
			if sub == subscriber {
				e.subscribers[eventType] = append(subscribers[:i], subscribers[i+1:]...)
				subscriber.Quit <- true
				log.Printf("\nSubscriber %s is unsubscribed for event %s\n", subscriber.Name, eventType)
				break
			}
		}

		if len(e.subscribers[eventType]) == 0 {
			delete(e.subscribers, eventType)
		}
	}
}
