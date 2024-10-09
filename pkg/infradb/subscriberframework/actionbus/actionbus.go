// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2023-2024 Intel Corporation, or its subsidiaries.
// Copyright (c) 2024 Ericsson AB

// Package actionbus holds implementation for subscribing and receiving actions
package actionbus

import (
	"fmt"
	"log"
	"sync"

	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

// ABus holds the ActionBus object
var ABus = NewActionBus()

// ActionBus holds the action bus info
type ActionBus struct {
	subscribers    map[string][]*Subscriber
	actionHandlers map[string]ActionHandler
	subscriberL    sync.RWMutex
}

// Subscriber holds the info for each subscriber
type Subscriber struct {
	Name string
	Ch   chan interface{}
	Quit chan bool
}

// ActionHandler handles the action requests that arrive
type ActionHandler interface {
	HandleAction(string, *ActionData)
}

// ActionData holds the data for each action
type ActionData struct {
	ErrCh chan error
}

// StartSubscriber will be called by the modules to initialize and start listening for actions
func (a *ActionBus) StartSubscriber(moduleName, actionType string, actionHandler ActionHandler) {
	if !a.subscriberExist(actionType, moduleName) {
		subscriber := a.Subscribe(moduleName, actionType, actionHandler)

		go func() {
			for {
				select {
				case action := <-subscriber.Ch:
					log.Printf("\nSubscriber %s for %s received \n", moduleName, actionType)

					handlerKey := utils.ComposeHandlerName(moduleName, actionType)
					if handler, ok := a.actionHandlers[handlerKey]; ok {
						if actionData, ok := action.(*ActionData); ok {
							handler.HandleAction(actionType, actionData)
						} else {
							log.Println("error: unexpected action type")
						}
					} else {
						log.Println("error: no action handler found")
					}
				case <-subscriber.Quit:
					close(subscriber.Ch)
					return
				}
			}
		}()
	}
}

// NewActionBus initializes an ActionBus object
func NewActionBus() *ActionBus {
	return &ActionBus{
		subscribers:    make(map[string][]*Subscriber),
		actionHandlers: make(map[string]ActionHandler),
	}
}

// NewActionData initializes an ActionData object
func NewActionData() *ActionData {
	return &ActionData{
		ErrCh: make(chan error),
	}
}

// Subscribe api provides registration of a subscriber to the given action
func (a *ActionBus) Subscribe(moduleName, actionType string, actionHandler ActionHandler) *Subscriber {
	a.subscriberL.Lock()
	defer a.subscriberL.Unlock()

	subscriber := &Subscriber{
		Name: moduleName,
		Ch:   make(chan interface{}),
		Quit: make(chan bool),
	}

	a.subscribers[actionType] = append(a.subscribers[actionType], subscriber)

	handlerKey := utils.ComposeHandlerName(moduleName, actionType)
	a.actionHandlers[handlerKey] = actionHandler

	log.Printf("Subscriber %s registered for action %s\n", moduleName, actionType)
	return subscriber
}

// GetSubscribers api is used to fetch the list of subscribers registered with given actionType
func (a *ActionBus) GetSubscribers(actionType string) []*Subscriber {
	a.subscriberL.RLock()
	defer a.subscriberL.RUnlock()

	return a.subscribers[actionType]
}

// Publish api notifies the subscribers with certain actionType
func (a *ActionBus) Publish(actionData *ActionData, subscriber *Subscriber) error {
	var err error

	select {
	case subscriber.Ch <- actionData:
		log.Printf("Publish(): Notification is sent to subscriber %s\n", subscriber.Name)
	default:
		err = fmt.Errorf("channel for subscriber %s is busy", subscriber.Name)
	}
	return err
}

func (a *ActionBus) subscriberExist(actionType string, moduleName string) bool {
	subList := a.GetSubscribers(actionType)
	if len(subList) != 0 {
		for _, s := range subList {
			if s.Name == moduleName {
				return true
			}
		}
	}
	return false
}
