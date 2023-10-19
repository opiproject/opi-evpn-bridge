// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package svi is the main package of the application
package svi

import (
	"testing"

	"github.com/philippgille/gokv"
	"github.com/philippgille/gokv/gomap"

	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

func TestFrontEnd_NewServerWithArgs(t *testing.T) {
	tests := map[string]struct {
		frr       utils.Frr
		nLink     utils.Netlink
		store     gokv.Store
		wantPanic bool
	}{
		"nil netlink argument": {
			frr:       &utils.FrrWrapper{},
			nLink:     nil,
			store:     gomap.NewStore(gomap.DefaultOptions),
			wantPanic: true,
		},
		"nil store argument": {
			frr:       &utils.FrrWrapper{},
			nLink:     &utils.NetlinkWrapper{},
			store:     nil,
			wantPanic: true,
		},
		"nil frr argument": {
			frr:       nil,
			nLink:     &utils.NetlinkWrapper{},
			store:     gomap.NewStore(gomap.DefaultOptions),
			wantPanic: true,
		},
		"all valid arguments": {
			frr:       &utils.FrrWrapper{},
			nLink:     &utils.NetlinkWrapper{},
			store:     gomap.NewStore(gomap.DefaultOptions),
			wantPanic: false,
		},
	}

	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			defer func() {
				r := recover()
				if (r != nil) != tt.wantPanic {
					t.Errorf("NewServerWithArgs() recover = %v, wantPanic = %v", r, tt.wantPanic)
				}
			}()

			server := NewServerWithArgs(tt.nLink, tt.frr, tt.store)
			if server == nil && !tt.wantPanic {
				t.Error("expected non nil server or panic")
			}
		})
	}
}
