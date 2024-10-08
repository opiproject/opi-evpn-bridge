// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2023-2024 Intel Corporation, or its subsidiaries.
// Copyright (c) 2024 Ericsson AB.

package infradb

import (
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/common"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb/subscriberframework/eventbus"
)

// IDomain holds the utilities for child objects
type IDomain interface {
	getStatusComponents() []common.Component
	setStatusComponents([]common.Component)
	isOperationalStatus(OperStatus) bool
	setOperationalStatus(OperStatus)
	setNewResourceVersion()
}

// OperStatus operational Status
type OperStatus int32

const (
	// OperStatusUnspecified for "unknown" state
	OperStatusUnspecified OperStatus = iota
	// OperStatusUp for "up" state
	OperStatusUp = iota
	// OperStatusDown for "down" state
	OperStatusDown = iota
	// OperStatusToBeDeleted for "to be deleted" state
	OperStatusToBeDeleted = iota
)

// Domain holds domain info
type Domain struct {
	IDomain IDomain
}

// prepareObjectsForReplay prepares an object for replay by setting the unsuccessful components
// in pending state and returning a list of the components that need to be contacted for the
// replay of the particular object that called the function.
func (in *Domain) prepareObjectsForReplay(componentName string, vrfSubs []*eventbus.Subscriber) []*eventbus.Subscriber {
	// We assume that the list of Components that are returned
	// from DB is ordered based on the priority as that was the
	// way that has been stored in the DB in first place.

	vrfComponents := in.IDomain.getStatusComponents()
	auxComponents := in.IDomain.getStatusComponents()
	tempSubs := []*eventbus.Subscriber{}
	for i, comp := range vrfComponents {
		if comp.Name == componentName || comp.CompStatus != common.ComponentStatusSuccess {
			auxComponents[i] = common.Component{Name: comp.Name, CompStatus: common.ComponentStatusPending, Details: ""}
			tempSubs = append(tempSubs, vrfSubs[i])
		}
	}

	in.IDomain.setStatusComponents(auxComponents)

	if in.IDomain.isOperationalStatus(OperStatusUp) {
		in.IDomain.setOperationalStatus(OperStatusDown)
	}

	in.IDomain.setNewResourceVersion()
	return tempSubs
}
