// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

// Package evpn is the main package of the application
package evpn

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"path"
	"sort"

	"github.com/google/uuid"
	"github.com/vishvananda/netlink"

	pb "github.com/opiproject/opi-api/network/evpn-gw/v1alpha1/gen/go"

	"go.einride.tech/aip/fieldbehavior"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/resourceid"
	"go.einride.tech/aip/resourcename"
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
func (s *Server) CreateSvi(_ context.Context, in *pb.CreateSviRequest) (*pb.Svi, error) {
	log.Printf("CreateSvi: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.SviId != "" {
		err := resourceid.ValidateUserSettable(in.SviId)
		if err != nil {
			log.Printf("error: %v", err)
			return nil, err
		}
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
	// Validate that a LogicalBridge resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Svi.Spec.LogicalBridge); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a Vrf resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Svi.Spec.Vrf); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// now get LogicalBridge object to fetch VID field
	bridgeObject, ok := s.Bridges[in.Svi.Spec.LogicalBridge]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Svi.Spec.LogicalBridge)
		log.Printf("error: %v", err)
		return nil, err
	}
	// now get Vrf to plug this vlandev into
	vrf, ok := s.Vrfs[in.Svi.Spec.Vrf]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Svi.Spec.Vrf)
		log.Printf("error: %v", err)
		return nil, err
	}
	// not found, so create a new one
	bridge, err := s.nLink.LinkByName(tenantbridgeName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", tenantbridgeName)
		log.Printf("error: %v", err)
		return nil, err
	}
	vid := uint16(bridgeObject.Spec.VlanId)
	// Example: bridge vlan add dev br-tenant vid <vlan-id> self
	if err := s.nLink.BridgeVlanAdd(bridge, vid, false, false, true, false); err != nil {
		fmt.Printf("Failed to add vlan to bridge: %v", err)
		return nil, err
	}
	// Example: ip link add link br-tenant name <link_svi> type vlan id <vlan-id>
	vlanName := fmt.Sprintf("vlan%d", vid)
	vlandev := &netlink.Vlan{LinkAttrs: netlink.LinkAttrs{Name: vlanName, ParentIndex: bridge.Attrs().Index}, VlanId: int(vid)}
	log.Printf("Creating VLAN %v", vlandev)
	if err := s.nLink.LinkAdd(vlandev); err != nil {
		fmt.Printf("Failed to create vlan link: %v", err)
		return nil, err
	}
	// Example: ip link set <link_svi> addr aa:bb:cc:00:00:41
	if len(in.Svi.Spec.MacAddress) > 0 {
		if err := s.nLink.LinkSetHardwareAddr(vlandev, in.Svi.Spec.MacAddress); err != nil {
			fmt.Printf("Failed to set MAC on link: %v", err)
			return nil, err
		}
	}
	// Example: ip address add <svi-ip-with prefixlength> dev <link_svi>
	for _, gwip := range in.Svi.Spec.GwIpPrefix {
		fmt.Printf("Assign the GW IP address %v to the SVI interface %v", gwip, vlandev)
		myip := make(net.IP, 4)
		binary.BigEndian.PutUint32(myip, gwip.Addr.GetV4Addr())
		addr := &netlink.Addr{IPNet: &net.IPNet{IP: myip, Mask: net.CIDRMask(int(gwip.Len), 32)}}
		if err := s.nLink.AddrAdd(vlandev, addr); err != nil {
			fmt.Printf("Failed to set IP on link: %v", err)
			return nil, err
		}
	}
	// get net device by name
	vrfdev, err := s.nLink.LinkByName(path.Base(vrf.Name))
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", vrf.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// Example: ip link set <link_svi> master <vrf-name> up
	if err := s.nLink.LinkSetMaster(vlandev, vrfdev); err != nil {
		fmt.Printf("Failed to add vlandev to vrf: %v", err)
		return nil, err
	}
	// Example: ip link set <link_svi> up
	if err := s.nLink.LinkSetUp(vlandev); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	response := protoClone(in.Svi)
	response.Status = &pb.SviStatus{OperStatus: pb.SVIOperStatus_SVI_OPER_STATUS_UP}
	s.Svis[in.Svi.Name] = response
	log.Printf("CreateSvi: Sending to client: %v", response)
	return response, nil
}

// DeleteSvi deletes a VLAN
func (s *Server) DeleteSvi(_ context.Context, in *pb.DeleteSviRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteSvi: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	obj, ok := s.Svis[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// use netlink to find br-tenant
	bridge, err := s.nLink.LinkByName(tenantbridgeName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", tenantbridgeName)
		log.Printf("error: %v", err)
		return nil, err
	}
	// use netlink to find VlanId from LogicalBridge object
	bridgeObject, ok := s.Bridges[obj.Spec.LogicalBridge]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", obj.Spec.LogicalBridge)
		log.Printf("error: %v", err)
		return nil, err
	}
	vid := uint16(bridgeObject.Spec.VlanId)
	// Example: bridge vlan del dev br-tenant vid <vlan-id> self
	if err := s.nLink.BridgeVlanDel(bridge, vid, false, false, true, false); err != nil {
		fmt.Printf("Failed to del vlan to bridge: %v", err)
		return nil, err
	}
	vlanName := fmt.Sprintf("vlan%d", vid)
	vlandev, err := s.nLink.LinkByName(vlanName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", vlanName)
		log.Printf("error: %v", err)
		return nil, err
	}
	log.Printf("Deleting VLAN %v", vlandev)
	// bring link down
	if err := s.nLink.LinkSetDown(vlandev); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	// use netlink to delete vlan
	if err := s.nLink.LinkDel(vlandev); err != nil {
		fmt.Printf("Failed to delete link: %v", err)
		return nil, err
	}
	// remove from the Database
	delete(s.Svis, obj.Name)
	return &emptypb.Empty{}, nil
}

// UpdateSvi updates an VLAN
func (s *Server) UpdateSvi(_ context.Context, in *pb.UpdateSviRequest) (*pb.Svi, error) {
	log.Printf("UpdateSvi: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Svi.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	svi, ok := s.Svis[in.Svi.Name]
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Svi.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.Svi); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// use netlink to find VlanId from LogicalBridge object
	bridgeObject, ok := s.Bridges[svi.Spec.LogicalBridge]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", svi.Spec.LogicalBridge)
		log.Printf("error: %v", err)
		return nil, err
	}
	vlanName := fmt.Sprintf("vlan%d", bridgeObject.Spec.VlanId)
	iface, err := s.nLink.LinkByName(vlanName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", vlanName)
		log.Printf("error: %v", err)
		return nil, err
	}
	// base := iface.Attrs()
	// iface.MTU = 1500 // TODO: remove this, just an example
	if err := s.nLink.LinkModify(iface); err != nil {
		fmt.Printf("Failed to update link: %v", err)
		return nil, err
	}
	response := protoClone(in.Svi)
	response.Status = &pb.SviStatus{OperStatus: pb.SVIOperStatus_SVI_OPER_STATUS_UP}
	s.Svis[in.Svi.Name] = response
	log.Printf("UpdateSvi: Sending to client: %v", response)
	return response, nil
}

// GetSvi gets an VLAN
func (s *Server) GetSvi(_ context.Context, in *pb.GetSviRequest) (*pb.Svi, error) {
	log.Printf("GetSvi: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	obj, ok := s.Svis[in.Name]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// use netlink to find VlanId from LogicalBridge object
	bridgeObject, ok := s.Bridges[obj.Spec.LogicalBridge]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", obj.Spec.LogicalBridge)
		log.Printf("error: %v", err)
		return nil, err
	}
	vlanName := fmt.Sprintf("vlan%d", bridgeObject.Spec.VlanId)
	_, err := s.nLink.LinkByName(vlanName)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", vlanName)
		log.Printf("error: %v", err)
		return nil, err
	}
	// TODO
	return &pb.Svi{Name: in.Name, Spec: &pb.SviSpec{MacAddress: obj.Spec.MacAddress, EnableBgp: obj.Spec.EnableBgp, RemoteAs: obj.Spec.RemoteAs}, Status: &pb.SviStatus{OperStatus: pb.SVIOperStatus_SVI_OPER_STATUS_UP}}, nil
}

// ListSvis lists logical bridges
func (s *Server) ListSvis(_ context.Context, in *pb.ListSvisRequest) (*pb.ListSvisResponse, error) {
	log.Printf("ListSvis: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch pagination from the database, calculate size and offset
	size, offset, perr := extractPagination(in.PageSize, in.PageToken, s.Pagination)
	if perr != nil {
		log.Printf("error: %v", perr)
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
