// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package svi is the main package of the application
package svi

import (
	"context"
	"fmt"
	"log"
	"path"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/opiproject/opi-evpn-bridge/pkg/models"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/resourceid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func sortSvis(svis []*pb.Svi) {
	sort.Slice(svis, func(i int, j int) bool {
		return svis[i].Name < svis[j].Name
	})
}

// CreateSvi executes the creation of the VLAN
func (s *Server) CreateSvi(ctx context.Context, in *pb.CreateSviRequest) (*pb.Svi, error) {
	// check input correctness
	if err := s.validateCreateSviRequest(in); err != nil {
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.SviId != "" {
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.SviId, in.Svi.Name)
		resourceID = in.SviId
	}
	in.Svi.Name = resourceIDToFullName("svis", resourceID)
	// idempotent API when called with same key, should return same object
	obj := new(pb.Svi)
	ok, err := s.store.Get(in.Svi.Name, obj)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if ok {
		log.Printf("Already existing Svi with id %v", in.Svi.Name)
		return obj, nil
	}
	// now get LogicalBridge object to fetch VID field
	bridgeObject := new(pb.LogicalBridge)
	ok, err = s.store.Get(in.Svi.Spec.LogicalBridge, bridgeObject)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Svi.Spec.LogicalBridge)
		return nil, err
	}
	// now get Vrf to plug this vlandev into
	vrf := new(pb.Vrf)
	ok, err = s.store.Get(in.Svi.Spec.Vrf, vrf)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Svi.Spec.Vrf)
		return nil, err
	}
	// configure netlink
	if err := s.netlinkCreateSvi(ctx, in, bridgeObject, vrf); err != nil {
		return nil, err
	}
	// configure FRR
	vid := uint16(bridgeObject.Spec.VlanId)
	vlanName := fmt.Sprintf("vlan%d", vid)
	vrfName := path.Base(vrf.Name)
	if err := s.frrCreateSviRequest(ctx, in, vrfName, vlanName); err != nil {
		return nil, err
	}
	// translate object
	response := protoClone(in.Svi)
	response.Status = &pb.SviStatus{OperStatus: pb.SVIOperStatus_SVI_OPER_STATUS_UP}
	log.Printf("new object %v", models.NewSvi(response))
	// save object to the database
	s.ListHelper[in.Svi.Name] = false
	err = s.store.Set(in.Svi.Name, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// DeleteSvi deletes a VLAN
func (s *Server) DeleteSvi(ctx context.Context, in *pb.DeleteSviRequest) (*emptypb.Empty, error) {
	// check input correctness
	if err := s.validateDeleteSviRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	obj := new(pb.Svi)
	ok, err := s.store.Get(in.Name, obj)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return nil, err
	}
	// fetch object from the database
	bridgeObject := new(pb.LogicalBridge)
	ok, err = s.store.Get(obj.Spec.LogicalBridge, bridgeObject)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", obj.Spec.LogicalBridge)
		return nil, err
	}
	// fetch object from the database
	vrf := new(pb.Vrf)
	ok, err = s.store.Get(obj.Spec.Vrf, vrf)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", obj.Spec.Vrf)
		return nil, err
	}
	// configure netlink
	if err := s.netlinkDeleteSvi(ctx, in, bridgeObject, vrf); err != nil {
		return nil, err
	}
	// delete from FRR
	vrfName := path.Base(vrf.Name)
	vid := uint16(bridgeObject.Spec.VlanId)
	vlanName := fmt.Sprintf("vlan%d", vid)
	if err := s.frrDeleteSviRequest(ctx, obj, vrfName, vlanName); err != nil {
		return nil, err
	}
	// remove from the Database
	delete(s.ListHelper, obj.Name)
	err = s.store.Delete(obj.Name)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// UpdateSvi updates an VLAN
func (s *Server) UpdateSvi(ctx context.Context, in *pb.UpdateSviRequest) (*pb.Svi, error) {
	// check input correctness
	if err := s.validateUpdateSviRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	svi := new(pb.Svi)
	ok, err := s.store.Get(in.Svi.Name, svi)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Svi.Name)
		return nil, err
	}
	// use netlink to find VlanId from LogicalBridge object
	bridgeObject := new(pb.LogicalBridge)
	ok, err = s.store.Get(svi.Spec.LogicalBridge, bridgeObject)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", svi.Spec.LogicalBridge)
		return nil, err
	}
	vlanName := fmt.Sprintf("vlan%d", bridgeObject.Spec.VlanId)
	iface, err := s.nLink.LinkByName(ctx, vlanName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", vlanName)
		return nil, err
	}
	// base := iface.Attrs()
	// iface.MTU = 1500 // TODO: remove this, just an example
	if err := s.nLink.LinkModify(ctx, iface); err != nil {
		fmt.Printf("Failed to update link: %v", err)
		return nil, err
	}
	response := protoClone(in.Svi)
	response.Status = &pb.SviStatus{OperStatus: pb.SVIOperStatus_SVI_OPER_STATUS_UP}
	err = s.store.Set(in.Svi.Name, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetSvi gets an VLAN
func (s *Server) GetSvi(ctx context.Context, in *pb.GetSviRequest) (*pb.Svi, error) {
	// check input correctness
	if err := s.validateGetSviRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	obj := new(pb.Svi)
	ok, err := s.store.Get(in.Name, obj)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return nil, err
	}
	// use netlink to find VlanId from LogicalBridge object
	bridgeObject := new(pb.LogicalBridge)
	ok, err = s.store.Get(obj.Spec.LogicalBridge, bridgeObject)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", obj.Spec.LogicalBridge)
		return nil, err
	}
	vlanName := fmt.Sprintf("vlan%d", bridgeObject.Spec.VlanId)
	_, err = s.nLink.LinkByName(ctx, vlanName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", vlanName)
		return nil, err
	}
	// TODO
	return &pb.Svi{Name: in.Name, Spec: &pb.SviSpec{MacAddress: obj.Spec.MacAddress, EnableBgp: obj.Spec.EnableBgp, RemoteAs: obj.Spec.RemoteAs}, Status: &pb.SviStatus{OperStatus: pb.SVIOperStatus_SVI_OPER_STATUS_UP}}, nil
}

// ListSvis lists logical bridges
func (s *Server) ListSvis(_ context.Context, in *pb.ListSvisRequest) (*pb.ListSvisResponse, error) {
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		return nil, err
	}
	// fetch pagination from the database, calculate size and offset
	size, offset, perr := extractPagination(in.PageSize, in.PageToken, s.Pagination)
	if perr != nil {
		return nil, perr
	}
	// fetch object from the database
	Blobarray := []*pb.Svi{}
	for key := range s.ListHelper {
		if !strings.HasPrefix(key, "//network.opiproject.org/svis") {
			continue
		}
		svi := new(pb.Svi)
		ok, err := s.store.Get(key, svi)
		if err != nil {
			fmt.Printf("Failed to interact with store: %v", err)
			return nil, err
		}
		if !ok {
			err := status.Errorf(codes.NotFound, "unable to find key %s", key)
			return nil, err
		}
		Blobarray = append(Blobarray, svi)
	}
	// sort is needed, since MAP is unsorted in golang, and we might get different results
	sortSvis(Blobarray)
	log.Printf("Limiting result len(%d) to [%d:%d]", len(Blobarray), offset, size)
	Blobarray, hasMoreElements := limitPagination(Blobarray, offset, size)
	token := ""
	if hasMoreElements {
		token = uuid.New().String()
		s.Pagination[token] = offset + size
	}
	return &pb.ListSvisResponse{Svis: Blobarray, NextPageToken: token}, nil
}
