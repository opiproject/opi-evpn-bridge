// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"fmt"
	"log"
	"math"
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

func sortVrfs(vrfs []*pb.Vrf) {
	sort.Slice(vrfs, func(i int, j int) bool {
		return vrfs[i].Name < vrfs[j].Name
	})
}

// CreateVrf executes the creation of the VRF
func (s *Server) CreateVrf(ctx context.Context, in *pb.CreateVrfRequest) (*pb.Vrf, error) {
	// check input correctness
	if err := s.validateCreateVrfRequest(in); err != nil {
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.VrfId != "" {
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.VrfId, in.Vrf.Name)
		resourceID = in.VrfId
	}
	in.Vrf.Name = resourceIDToFullName("vrfs", resourceID)
	// idempotent API when called with same key, should return same object
	obj := new(pb.Vrf)
	ok, err := s.store.Get(in.Vrf.Name, obj)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if ok {
		log.Printf("Already existing Vrf with id %v", in.Vrf.Name)
		return obj, nil
	}
	// TODO: consider choosing random table ID
	tableID := uint32(1000)
	if in.Vrf.Spec.Vni != nil {
		tableID = uint32(1001 + math.Mod(float64(*in.Vrf.Spec.Vni), 10.0))
	}
	// generate random mac, since it is not part of user facing API
	mac, err := generateRandMAC()
	if err != nil {
		fmt.Printf("Failed to generate random MAC: %v", err)
		return nil, err
	}
	// configure netlink
	if err := s.netlinkCreateVrf(ctx, in, tableID, mac); err != nil {
		return nil, err
	}
	// configure FRR
	if err := s.frrCreateVrfRequest(ctx, in); err != nil {
		return nil, err
	}
	// translate object
	response := protoClone(in.Vrf)
	response.Status = &pb.VrfStatus{LocalAs: 4, RoutingTable: tableID, Rmac: mac}
	log.Printf("new object %v", models.NewVrf(response))
	// save object to the database
	s.ListHelper[in.Vrf.Name] = false
	err = s.store.Set(in.Vrf.Name, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// DeleteVrf deletes a VRF
func (s *Server) DeleteVrf(ctx context.Context, in *pb.DeleteVrfRequest) (*emptypb.Empty, error) {
	// check input correctness
	if err := s.validateDeleteVrfRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	obj := new(pb.Vrf)
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
	// configure netlink
	if err := s.netlinkDeleteVrf(ctx, obj); err != nil {
		return nil, err
	}
	// delete from FRR
	if err := s.frrDeleteVrfRequest(ctx, obj); err != nil {
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

// UpdateVrf updates an VRF
func (s *Server) UpdateVrf(ctx context.Context, in *pb.UpdateVrfRequest) (*pb.Vrf, error) {
	// check input correctness
	if err := s.validateUpdateVrfRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	obj := new(pb.Vrf)
	ok, err := s.store.Get(in.Vrf.Name, obj)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Vrf.Name)
		return nil, err
	}
	resourceID := path.Base(obj.Name)
	iface, err := s.nLink.LinkByName(ctx, resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		return nil, err
	}
	// base := iface.Attrs()
	// iface.MTU = 1500 // TODO: remove this, just an example
	if err := s.nLink.LinkModify(ctx, iface); err != nil {
		fmt.Printf("Failed to update link: %v", err)
		return nil, err
	}
	response := protoClone(in.Vrf)
	response.Status = &pb.VrfStatus{LocalAs: 4}
	err = s.store.Set(in.Vrf.Name, response)
	if err != nil {
		return nil, err
	}
	return response, nil
}

// GetVrf gets an VRF
func (s *Server) GetVrf(ctx context.Context, in *pb.GetVrfRequest) (*pb.Vrf, error) {
	// check input correctness
	if err := s.validateGetVrfRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	obj := new(pb.Vrf)
	ok, err := s.store.Get(in.Name, obj)
	if err != nil {
		fmt.Printf("Failed to interact with store: %v", err)
		return nil, err
	}
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return nil, err
	}
	resourceID := path.Base(obj.Name)
	_, err = s.nLink.LinkByName(ctx, resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		return nil, err
	}
	// TODO
	return &pb.Vrf{Name: in.Name, Spec: &pb.VrfSpec{Vni: obj.Spec.Vni}, Status: &pb.VrfStatus{LocalAs: 77}}, nil
}

// ListVrfs lists logical bridges
func (s *Server) ListVrfs(_ context.Context, in *pb.ListVrfsRequest) (*pb.ListVrfsResponse, error) {
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
	Blobarray := []*pb.Vrf{}
	for key := range s.ListHelper {
		if !strings.HasPrefix(key, "//network.opiproject.org/vrfs") {
			continue
		}
		vrf := new(pb.Vrf)
		ok, err := s.store.Get(key, vrf)
		if err != nil {
			fmt.Printf("Failed to interact with store: %v", err)
			return nil, err
		}
		if !ok {
			err := status.Errorf(codes.NotFound, "unable to find key %s", key)
			return nil, err
		}
		Blobarray = append(Blobarray, vrf)
	}
	// sort is needed, since MAP is unsorted in golang, and we might get different results
	sortVrfs(Blobarray)
	log.Printf("Limiting result len(%d) to [%d:%d]", len(Blobarray), offset, size)
	Blobarray, hasMoreElements := limitPagination(Blobarray, offset, size)
	token := ""
	if hasMoreElements {
		token = uuid.New().String()
		s.Pagination[token] = offset + size
	}
	return &pb.ListVrfsResponse{Vrfs: Blobarray, NextPageToken: token}, nil
}
