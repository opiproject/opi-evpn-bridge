// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"fmt"
	"log"
	"path"
	"sort"

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
	obj, ok := s.Svis[in.Svi.Name]
	if ok {
		log.Printf("Already existing Svi with id %v", in.Svi.Name)
		return obj, nil
	}
	// now get LogicalBridge object to fetch VID field
	bridgeObject, ok := s.Bridges[in.Svi.Spec.LogicalBridge]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Svi.Spec.LogicalBridge)
		return nil, err
	}
	// now get Vrf to plug this vlandev into
	vrf, ok := s.Vrfs[in.Svi.Spec.Vrf]
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
	s.Svis[in.Svi.Name] = response
	return response, nil
}

// DeleteSvi deletes a VLAN
func (s *Server) DeleteSvi(ctx context.Context, in *pb.DeleteSviRequest) (*emptypb.Empty, error) {
	// check input correctness
	if err := s.validateDeleteSviRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	obj, ok := s.Svis[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return nil, err
	}
	// fetch object from the database
	bridgeObject, ok := s.Bridges[obj.Spec.LogicalBridge]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", obj.Spec.LogicalBridge)
		return nil, err
	}
	// fetch object from the database
	vrf, ok := s.Vrfs[obj.Spec.Vrf]
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
	delete(s.Svis, obj.Name)
	return &emptypb.Empty{}, nil
}

// UpdateSvi updates an VLAN
func (s *Server) UpdateSvi(ctx context.Context, in *pb.UpdateSviRequest) (*pb.Svi, error) {
	// check input correctness
	if err := s.validateUpdateSviRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	svi, ok := s.Svis[in.Svi.Name]
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Svi.Name)
		return nil, err
	}
	// use netlink to find VlanId from LogicalBridge object
	bridgeObject, ok := s.Bridges[svi.Spec.LogicalBridge]
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
	s.Svis[in.Svi.Name] = response
	return response, nil
}

// GetSvi gets an VLAN
func (s *Server) GetSvi(ctx context.Context, in *pb.GetSviRequest) (*pb.Svi, error) {
	// check input correctness
	if err := s.validateGetSviRequest(in); err != nil {
		return nil, err
	}
	// fetch object from the database
	obj, ok := s.Svis[in.Name]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		return nil, err
	}
	// use netlink to find VlanId from LogicalBridge object
	bridgeObject, ok := s.Bridges[obj.Spec.LogicalBridge]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", obj.Spec.LogicalBridge)
		return nil, err
	}
	vlanName := fmt.Sprintf("vlan%d", bridgeObject.Spec.VlanId)
	_, err := s.nLink.LinkByName(ctx, vlanName)
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
	for _, svi := range s.Svis {
		r := protoClone(svi)
		r.Status = &pb.SviStatus{OperStatus: pb.SVIOperStatus_SVI_OPER_STATUS_UP}
		Blobarray = append(Blobarray, r)
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
