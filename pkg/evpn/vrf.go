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
	"math"
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

func sortVrfs(vrfs []*pb.Vrf) {
	sort.Slice(vrfs, func(i int, j int) bool {
		return vrfs[i].Name < vrfs[j].Name
	})
}

// CreateVrf executes the creation of the VRF
func (s *Server) CreateVrf(_ context.Context, in *pb.CreateVrfRequest) (*pb.Vrf, error) {
	log.Printf("CreateVrf: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// see https://google.aip.dev/133#user-specified-ids
	resourceID := resourceid.NewSystemGenerated()
	if in.VrfId != "" {
		err := resourceid.ValidateUserSettable(in.VrfId)
		if err != nil {
			log.Printf("error: %v", err)
			return nil, err
		}
		log.Printf("client provided the ID of a resource %v, ignoring the name field %v", in.VrfId, in.Vrf.Name)
		resourceID = in.VrfId
	}
	in.Vrf.Name = resourceIDToFullName("vrfs", resourceID)
	// idempotent API when called with same key, should return same object
	obj, ok := s.Vrfs[in.Vrf.Name]
	if ok {
		log.Printf("Already existing Vrf with id %v", in.Vrf.Name)
		return obj, nil
	}
	// not found, so create a new one
	vrfName := resourceID
	// TODO: consider choosing random table ID
	tableID := uint32(1000)
	if in.Vrf.Spec.Vni != nil {
		tableID = uint32(1001 + math.Mod(float64(*in.Vrf.Spec.Vni), 10.0))
	}
	// Example: ip link add blue type vrf table 1000
	vrf := &netlink.Vrf{LinkAttrs: netlink.LinkAttrs{Name: vrfName}, Table: tableID}
	log.Printf("Creating VRF %v", vrf)
	if err := s.nLink.LinkAdd(vrf); err != nil {
		fmt.Printf("Failed to create VRF link: %v", err)
		return nil, err
	}
	// Example: ip link set blue up
	if err := s.nLink.LinkSetUp(vrf); err != nil {
		fmt.Printf("Failed to up VRF link: %v", err)
		return nil, err
	}
	// Example: ip address add <vrf-loopback> dev <vrf-name>
	if in.Vrf.Spec.LoopbackIpPrefix != nil && in.Vrf.Spec.LoopbackIpPrefix.Addr != nil && in.Vrf.Spec.LoopbackIpPrefix.Len > 0 {
		myip := make(net.IP, 4)
		binary.BigEndian.PutUint32(myip, in.Vrf.Spec.LoopbackIpPrefix.Addr.GetV4Addr())
		addr := &netlink.Addr{IPNet: &net.IPNet{IP: myip, Mask: net.CIDRMask(int(in.Vrf.Spec.LoopbackIpPrefix.Len), 32)}}
		if err := s.nLink.AddrAdd(vrf, addr); err != nil {
			fmt.Printf("Failed to set IP on VRF link: %v", err)
			return nil, err
		}
	}
	// TODO: Add low-prio default route. Otherwise a miss leads to lookup in the next higher table
	// Example: ip route add throw default table <routing-table-number> proto evpn-gw-br metric 9999

	// generate random mac, since it is not part of user facing API
	mac, err := generateRandMAC()
	if err != nil {
		fmt.Printf("Failed to generate random MAC: %v", err)
		return nil, err
	}

	// create bridge and vxlan only if VNI value is not empty
	if in.Vrf.Spec.Vni != nil {
		// Example: ip link add br100 type bridge
		bridgeName := fmt.Sprintf("br%d", *in.Vrf.Spec.Vni)
		bridge := &netlink.Bridge{LinkAttrs: netlink.LinkAttrs{Name: bridgeName}}
		log.Printf("Creating Linux Bridge %v", bridge)
		if err := s.nLink.LinkAdd(bridge); err != nil {
			fmt.Printf("Failed to create Bridge link: %v", err)
			return nil, err
		}
		// Example: ip link set br100 master blue addrgenmode none
		if err := s.nLink.LinkSetMaster(bridge, vrf); err != nil {
			fmt.Printf("Failed to add Bridge to VRF: %v", err)
			return nil, err
		}
		// Example: ip link set br100 addr aa:bb:cc:00:00:02
		if err := s.nLink.LinkSetHardwareAddr(bridge, mac); err != nil {
			fmt.Printf("Failed to set MAC on Bridge link: %v", err)
			return nil, err
		}
		// Example: ip link set br100 up
		if err := s.nLink.LinkSetUp(bridge); err != nil {
			fmt.Printf("Failed to up Bridge link: %v", err)
			return nil, err
		}
		// Example: ip link add vni100 type vxlan local 10.0.0.4 dstport 4789 id 100 nolearning
		vxlanName := fmt.Sprintf("vni%d", *in.Vrf.Spec.Vni)
		myip := make(net.IP, 4)
		binary.BigEndian.PutUint32(myip, in.Vrf.Spec.VtepIpPrefix.Addr.GetV4Addr())
		// TODO: take Port from proto instead of hard-coded
		vxlan := &netlink.Vxlan{LinkAttrs: netlink.LinkAttrs{Name: vxlanName}, VxlanId: int(*in.Vrf.Spec.Vni), Port: 4789, Learning: false, SrcAddr: myip}
		log.Printf("Creating VXLAN %v", vxlan)
		if err := s.nLink.LinkAdd(vxlan); err != nil {
			fmt.Printf("Failed to create Vxlan link: %v", err)
			return nil, err
		}
		// Example: ip link set vni100 master br100 addrgenmode none
		if err := s.nLink.LinkSetMaster(vxlan, bridge); err != nil {
			fmt.Printf("Failed to add Vxlan to bridge: %v", err)
			return nil, err
		}
		// Example: ip link set vni100 up
		if err := s.nLink.LinkSetUp(vxlan); err != nil {
			fmt.Printf("Failed to up Vxlan link: %v", err)
			return nil, err
		}
	}
	response := protoClone(in.Vrf)
	response.Status = &pb.VrfStatus{LocalAs: 4, RoutingTable: tableID, Rmac: mac}
	s.Vrfs[in.Vrf.Name] = response
	log.Printf("CreateVrf: Sending to client: %v", response)
	return response, nil
}

// DeleteVrf deletes a VRF
func (s *Server) DeleteVrf(_ context.Context, in *pb.DeleteVrfRequest) (*emptypb.Empty, error) {
	log.Printf("DeleteVrf: Received from client: %v", in)
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
	obj, ok := s.Vrfs[in.Name]
	if !ok {
		if in.AllowMissing {
			return &emptypb.Empty{}, nil
		}
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// delete bridge and vxlan only if VNI value is not empty
	if obj.Spec.Vni != nil {
		// use netlink to find VXLAN device
		vxlanName := fmt.Sprintf("vni%d", *obj.Spec.Vni)
		vxlandev, err := s.nLink.LinkByName(vxlanName)
		log.Printf("Deleting VXLAN %v", vxlandev)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", vxlanName)
			log.Printf("error: %v", err)
			return nil, err
		}
		// bring link down
		if err := s.nLink.LinkSetDown(vxlandev); err != nil {
			fmt.Printf("Failed to up link: %v", err)
			return nil, err
		}
		// use netlink to delete VXLAN device
		if err := s.nLink.LinkDel(vxlandev); err != nil {
			fmt.Printf("Failed to delete link: %v", err)
			return nil, err
		}
		// use netlink to find BRIDGE device
		bridgeName := fmt.Sprintf("br%d", *obj.Spec.Vni)
		bridgedev, err := s.nLink.LinkByName(bridgeName)
		log.Printf("Deleting BRIDGE %v", bridgedev)
		if err != nil {
			err := status.Errorf(codes.NotFound, "unable to find key %s", bridgeName)
			log.Printf("error: %v", err)
			return nil, err
		}
		// bring link down
		if err := s.nLink.LinkSetDown(bridgedev); err != nil {
			fmt.Printf("Failed to up link: %v", err)
			return nil, err
		}
		// use netlink to delete BRIDGE device
		if err := s.nLink.LinkDel(bridgedev); err != nil {
			fmt.Printf("Failed to delete link: %v", err)
			return nil, err
		}
	}
	resourceID := path.Base(obj.Name)
	// use netlink to find VRF
	vrf, err := s.nLink.LinkByName(resourceID)
	log.Printf("Deleting VRF %v", vrf)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		log.Printf("error: %v", err)
		return nil, err
	}
	// bring link down
	if err := s.nLink.LinkSetDown(vrf); err != nil {
		fmt.Printf("Failed to up link: %v", err)
		return nil, err
	}
	// use netlink to delete VRF
	if err := s.nLink.LinkDel(vrf); err != nil {
		fmt.Printf("Failed to delete link: %v", err)
		return nil, err
	}
	// remove from the Database
	delete(s.Vrfs, obj.Name)
	return &emptypb.Empty{}, nil
}

// UpdateVrf updates an VRF
func (s *Server) UpdateVrf(_ context.Context, in *pb.UpdateVrfRequest) (*pb.Vrf, error) {
	log.Printf("UpdateVrf: Received from client: %v", in)
	// check required fields
	if err := fieldbehavior.ValidateRequiredFields(in); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// Validate that a resource name conforms to the restrictions outlined in AIP-122.
	if err := resourcename.Validate(in.Vrf.Name); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	// fetch object from the database
	vrf, ok := s.Vrfs[in.Vrf.Name]
	if !ok {
		// TODO: introduce "in.AllowMissing" field. In case "true", create a new resource, don't return error
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Vrf.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	// update_mask = 2
	if err := fieldmask.Validate(in.UpdateMask, in.Vrf); err != nil {
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(vrf.Name)
	iface, err := s.nLink.LinkByName(resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		log.Printf("error: %v", err)
		return nil, err
	}
	// base := iface.Attrs()
	// iface.MTU = 1500 // TODO: remove this, just an example
	if err := s.nLink.LinkModify(iface); err != nil {
		fmt.Printf("Failed to update link: %v", err)
		return nil, err
	}
	response := protoClone(in.Vrf)
	response.Status = &pb.VrfStatus{LocalAs: 4}
	s.Vrfs[in.Vrf.Name] = response
	log.Printf("UpdateVrf: Sending to client: %v", response)
	return response, nil
}

// GetVrf gets an VRF
func (s *Server) GetVrf(_ context.Context, in *pb.GetVrfRequest) (*pb.Vrf, error) {
	log.Printf("GetVrf: Received from client: %v", in)
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
	obj, ok := s.Vrfs[in.Name]
	if !ok {
		err := status.Errorf(codes.NotFound, "unable to find key %s", in.Name)
		log.Printf("error: %v", err)
		return nil, err
	}
	resourceID := path.Base(obj.Name)
	_, err := s.nLink.LinkByName(resourceID)
	if err != nil {
		err := status.Errorf(codes.NotFound, "unable to find key %s", resourceID)
		log.Printf("error: %v", err)
		return nil, err
	}
	// TODO
	return &pb.Vrf{Name: in.Name, Spec: &pb.VrfSpec{Vni: obj.Spec.Vni}, Status: &pb.VrfStatus{LocalAs: 77}}, nil
}

// ListVrfs lists logical bridges
func (s *Server) ListVrfs(_ context.Context, in *pb.ListVrfsRequest) (*pb.ListVrfsResponse, error) {
	log.Printf("ListVrfs: Received from client: %v", in)
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
	Blobarray := []*pb.Vrf{}
	for _, vrf := range s.Vrfs {
		r := protoClone(vrf)
		r.Status = &pb.VrfStatus{LocalAs: 4}
		Blobarray = append(Blobarray, r)
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
