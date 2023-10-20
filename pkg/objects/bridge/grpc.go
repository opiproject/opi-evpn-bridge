// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package bridge is the main package of the application
package bridge

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/opiproject/opi-evpn-bridge/pkg/models"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"go.einride.tech/aip/resourceid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Create executes the creation of the LogicalBridge
func (s *Server) Create(ctx context.Context, i any) (*models.Bridge, error) {
	// check input correctness
	in, err := s.validateCreateLogicalBridgeRequest(i)
	if err != nil {
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.LogicalBridgeId != "" {
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.LogicalBridgeId, in.LogicalBridge.Name)
		resourceID = in.LogicalBridgeId
	}
	in.LogicalBridge.Name = resourceIDToFullName(resourceID)
	// idempotent API when called with same key, should return same object
	obj := new(models.Bridge)
	ok, err := s.Store.Get(in.LogicalBridge.Name, obj)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if ok {
		log.Printf("Already existing LogicalBridge with id %v", in.LogicalBridge.Name)
		return obj, nil
	}
	// configure netlink
	if err := s.netlinkCreateLogicalBridge(ctx, in); err != nil {
		return nil, err
	}
	// translate object
	response := models.NewBridge(in.LogicalBridge)
	log.Printf("new object %v", response)
	// save object to the database
	s.ListHelper[in.LogicalBridge.Name] = false
	err = s.Store.Set(in.LogicalBridge.Name, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// Delete deletes a LogicalBridge
func (s *Server) Delete(ctx context.Context, i any) error {
	// check input correctness
	in, err := s.validateDeleteLogicalBridgeRequest(i)
	if err != nil {
		return err
	}
	// fetch object from the database
	obj := new(pb.LogicalBridge)
	ok, err := s.Store.Get(in.Name, obj)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return err
	}
	if !ok {
		if in.AllowMissing {
			return nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return err
	}
	// configure netlink
	if err := s.netlinkDeleteLogicalBridge(ctx, obj); err != nil {
		return err
	}
	// remove from the Database
	delete(s.ListHelper, obj.Name)
	err = s.Store.Delete(obj.Name)
	if err != nil {
		return err
	}
	return nil
}

// Update updates a LogicalBridge
func (s *Server) Update(ctx context.Context, i any) (*models.Bridge, error) {
	// check input correctness
	in, err := s.validateUpdateLogicalBridgeRequest(i)
	if err != nil {
		return nil, err
	}
	// fetch object from the database
	bridge := new(models.Bridge)
	ok, err := s.Store.Get(in.LogicalBridge.Name, bridge)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.LogicalBridge.Name)
		return nil, err
	}
	// only if VNI is not empty
	if bridge.Vni != 0 {
		vxlanName := fmt.Sprintf("vni%d", bridge.Vni)
		iface, err := s.NLink.LinkByName(ctx, vxlanName)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", vxlanName)
			return nil, err
		}
		// base := iface.Attrs()
		// iface.MTU = 1500 // TODO: remove this, just an example
		if err := s.NLink.LinkModify(ctx, iface); err != nil {
			fmt.Printf("Failed to update link: %v", err)
			return nil, err
		}
	}
	response := models.NewBridge(in.LogicalBridge)
	err = s.Store.Set(in.LogicalBridge.Name, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// Get gets a LogicalBridge
func (s *Server) Get(ctx context.Context, i any) (*models.Bridge, error) {
	// check input correctness
	in, err := s.validateGetLogicalBridgeRequest(i)
	if err != nil {
		return nil, err
	}
	// fetch object from the database
	bridge := new(models.Bridge)
	ok, err := s.Store.Get(in.Name, bridge)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return nil, err
	}
	// only if VNI is not empty
	if bridge.Vni != 0 {
		vxlanName := fmt.Sprintf("vni%d", bridge.Vni)
		_, err := s.NLink.LinkByName(ctx, vxlanName)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", vxlanName)
			return nil, err
		}
	}

	return bridge, nil
}

// List lists logical bridges
func (s *Server) List(_ context.Context, i any) ([]*models.Bridge, error) {
	// check required fields
	in, err := s.validateListLogicalBridgeRequest(i)
	if err != nil {
		return nil, err
	}

	// fetch pagination from the database, calculate size and offset
	size, offset, perr := utils.ExtractPagination(in.PageSize, in.PageToken, s.Pagination)
	if perr != nil {
		return nil, perr
	}
	// fetch object from the database
	var Blobarray []*models.Bridge
	for key := range s.ListHelper {
		if !strings.HasPrefix(key, "//network.opiproject.org/bridges") {
			continue
		}
		bridge := new(models.Bridge)
		ok, err := s.Store.Get(key, bridge)
		if err != nil {
			fmt.Printf("Failed to interact with store: %v", err)
			return nil, err
		}
		if !ok {
			err := status.Errorf(codes.NotFound, "unable to find key %s", key)
			return nil, err
		}
		Blobarray = append(Blobarray, bridge)
	}
	// sort is needed, since MAP is unsorted in golang, and we might get different results
	sortLogicalBridges(Blobarray)
	log.Printf("Limiting result len(%d) to [%d:%d]", len(Blobarray), offset, size)
	Blobarray, hasMoreElements := utils.LimitPagination(Blobarray, offset, size)
	token := ""
	if hasMoreElements {
		token = uuid.New().String()
		s.Pagination[token] = offset + size
	}
	return Blobarray, nil
}
