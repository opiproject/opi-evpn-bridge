package netlink

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	vn "github.com/vishvananda/netlink"
)

// annotateDBEntries annonates the database entries
func annotateDBEntries() {
	for _, nexthop := range latestNexthop {
		nexthop = nexthop.annotate()
		latestNexthop[nexthop.Key] = nexthop
	}
	for _, r := range latestRoutes {
		r = r.annotate()
		latestRoutes[r.Key] = r
	}

	for _, m := range latestFDB {
		m = m.annotate()
		latestFDB[m.Key] = m
	}
	for _, l2n := range latestL2Nexthop {
		l2n = l2n.annotate()
		latestL2Nexthop[l2n.Key] = l2n
	}
}

// nolint
func (l2n *L2NexthopStruct) annotate() *L2NexthopStruct {
	// Annotate certain L2 Nexthops with additional information from LB and GRD
	l2n.Metadata = make(map[interface{}]interface{})
	LB := l2n.lb
	if LB != nil {
		if l2n.Type == SVI {
			l2n.Metadata["vrf_id"] = *LB.Spec.Vni
		} else if l2n.Type == VXLAN {
			//# Remote EVPN MAC address learned on the VXLAN interface
			//# The L2 nexthop must have a destination IP address in dst
			l2n.Resolved = false
			l2n.Metadata["local_vtep_ip"] = *LB.Spec.VtepIP
			l2n.Metadata["remote_vtep_ip"] = l2n.Dst
			l2n.Metadata["vni"] = *LB.Spec.Vni
			//# The below physical nexthops are needed to transmit the VXLAN-encapsuleted packets
			//# directly from the nexthop table to a physical port (and avoid another recirculation
			//# for route lookup in the GRD table.)
			VRF, _ := infradb.GetVrf("//network.opiproject.org/vrfs/GRD")
			r, ok := lookupRoute(l2n.Dst, VRF)
			if ok {
				//  # For now pick the first physical nexthop (no ECMP yet)
				phyNh := r.Nexthops[0]
				link, _ := vn.LinkByName(NameIndex[phyNh.nexthop.LinkIndex])
				l2n.Metadata["phy_smac"] = link.Attrs().HardwareAddr.String()
				l2n.Metadata["egress_vport"] = phyPorts[NameIndex[phyNh.nexthop.LinkIndex]]
				if phyNh.Neighbor != nil {
					if phyNh.Neighbor.Type == PHY {
						l2n.Metadata["phy_dmac"] = phyNh.Neighbor.Neigh0.HardwareAddr.String()
						l2n.Resolved = true
					} else {
						log.Printf("netlink: Error: Neighbor type not PHY\n")
					}
				}
			}
		} else if l2n.Type == BRIDGEPORT {
			// BridgePort as L2 nexthop
			l2n.Metadata["vport_id"] = l2n.bp.Metadata.VPort
			l2n.Metadata["portType"] = l2n.bp.Spec.Ptype
		}
	}
	return l2n
}

// annotate the route
func (fdb *FdbEntryStruct) annotate() *FdbEntryStruct {
	if fdb.VlanID == 0 {
		return fdb
	}
	if fdb.lb != nil {
		return fdb
	}

	fdb.Metadata = make(map[interface{}]interface{})
	l2n := fdb.Nexthop
	if !reflect.ValueOf(l2n).IsZero() {
		fdb.Metadata["nh_id"] = l2n.ID
		if l2n.Type == VXLAN {
			fdbEntry := latestFDB[fdbKey{None, fdb.Mac}]
			l2n.Dst = fdbEntry.Nexthop.Dst
		}
		switch l2n.Type {
		case VXLAN:
			fdb.Metadata["direction"] = TX
		case BRIDGEPORT, SVI:
			fdb.Metadata["direction"] = RXTX

		default:
			fdb.Metadata["direction"] = None
		}
	}
	return fdb
}

// annotate function annonates the entries
func (route *RouteStruct) annotate() *RouteStruct {
	route.Metadata = make(map[interface{}]interface{})
	for i := 0; i < len(route.Nexthops); i++ {
		nexthop := route.Nexthops[i]
		route.Metadata["nh_ids"] = nexthop.ID
	}
	if route.Vrf.Spec.Vni != nil {
		route.Metadata["vrf_id"] = *route.Vrf.Spec.Vni
	} else {
		route.Metadata["vrf_id"] = 0
	}
	if len(route.Nexthops) != 0 {
		nexthop := route.Nexthops[0]
		if route.Vrf.Spec.Vni == nil { // GRD
			switch nexthop.NhType {
			case PHY:
				route.Metadata["direction"] = RXTX
			case ACC:
				route.Metadata["direction"] = RX
			default:
				route.Metadata["direction"] = None
			}
		} else {
			switch nexthop.NhType {
			case VXLAN:
				route.Metadata["direction"] = RXTX
			case SVI, ACC:
				route.Metadata["direction"] = RXTX
			default:
				route.Metadata["direction"] = None
			}
		}
	} else {
		route.Metadata["direction"] = None
	}
	return route
}

// nolint
func (nexthop *NexthopStruct) annotate() *NexthopStruct {
	nexthop.Metadata = make(map[interface{}]interface{})
	var phyFlag bool
	phyFlag = false
	for k := range phyPorts {
		if NameIndex[nexthop.nexthop.LinkIndex] == k {
			phyFlag = true
		}
	}
	if (nexthop.nexthop.Gw != nil && !nexthop.nexthop.Gw.IsUnspecified()) && nexthop.nexthop.LinkIndex != 0 && strings.HasPrefix(NameIndex[nexthop.nexthop.LinkIndex], path.Base(nexthop.Vrf.Name)+"-") && !nexthop.Local {
		nexthop.NhType = SVI
		link, _ := vn.LinkByName(NameIndex[nexthop.nexthop.LinkIndex])
		if nexthop.Neighbor != nil {
			if nexthop.Neighbor.Type == SVI {
				nexthop.NhType = SVI
				nexthop.Metadata["direction"] = RX
				nexthop.Metadata["smac"] = link.Attrs().HardwareAddr.String()
				nexthop.Metadata["dmac"] = nexthop.Neighbor.Neigh0.HardwareAddr.String()
				nexthop.Metadata["egress_vport"] = nexthop.Neighbor.Metadata["vport_id"]
				nexthop.Metadata["vlanID"] = nexthop.Neighbor.Metadata["vlanID"]
				nexthop.Metadata["portType"] = nexthop.Neighbor.Metadata["portType"]
			} else if nexthop.Neighbor.Type == VXLAN {
				nexthop.NhType = VXLAN
				nexthop.Metadata["direction"] = TX
				nexthop.Metadata["inner_dmac"] = nexthop.Neighbor.Neigh0.HardwareAddr.String()
				nexthop.Metadata["inner_smac"] = link.Attrs().HardwareAddr.String()
				L2N := nexthop.Neighbor.Metadata["l2_nh"].(L2NexthopStruct)
				if L2N.Resolved {
					nexthop.Metadata["local_vtep_ip"] = L2N.Metadata["local_vtep_ip"]
					nexthop.Metadata["remote_vtep_ip"] = L2N.Metadata["remote_vtep_ip"]
					nexthop.Metadata["vni"] = L2N.Metadata["vni"]
					nexthop.Metadata["phy_smac"] = L2N.Metadata["phy_smac"]
					nexthop.Metadata["phy_dmac"] = L2N.Metadata["phy_dmac"]
					nexthop.Metadata["egress_vport"] = L2N.Metadata["egress_vport"]
				} else {
					nexthop.Resolved = false
				}
			} else {
				nexthop.Resolved = false
				log.Printf("netlink: Failed to gather data for nexthop on physical port\n")
			}
		}
	} else if (nexthop.nexthop.Gw != nil && !nexthop.nexthop.Gw.IsUnspecified()) && phyFlag && !nexthop.Local {
		nexthop.NhType = PHY
		link1, _ := vn.LinkByName(NameIndex[nexthop.nexthop.LinkIndex])
		if link1 == nil {
			return nexthop
		}
		nexthop.Metadata["direction"] = TX
		nexthop.Metadata["smac"] = link1.Attrs().HardwareAddr.String()
		nexthop.Metadata["egress_vport"] = phyPorts[nexthop.nexthop.Gw.String()]
		if nexthop.Neighbor != nil {
			if nexthop.Neighbor.Type == PHY {
				nexthop.Metadata["dmac"] = nexthop.Neighbor.Neigh0.HardwareAddr.String()
			}
		} else {
			nexthop.Resolved = false
			log.Printf("netlink: Failed to gather data for nexthop on physical port")
		}
	} else if (nexthop.nexthop.Gw != nil && !nexthop.nexthop.Gw.IsUnspecified()) && NameIndex[nexthop.nexthop.LinkIndex] == fmt.Sprintf("br-%s", path.Base(nexthop.Vrf.Name)) && !nexthop.Local {
		nexthop.NhType = VXLAN
		G, _ := infradb.GetVrf(nexthop.Vrf.Name)
		var detail map[string]interface{}
		var Rmac net.HardwareAddr
		for _, com := range G.Status.Components {
			if com.Name == "frr" {
				err := json.Unmarshal([]byte(com.Details), &detail)
				if err != nil {
					log.Printf("netlink: Error: %v", err)
				}
				rmac, found := detail["rmac"].(string)
				if !found {
					log.Printf("netlink: Key 'rmac' not found")
					break
				}
				Rmac, err = net.ParseMAC(rmac)
				if err != nil {
					log.Printf("netlink: Error parsing MAC address: %v", err)
				}
			}
		}
		nexthop.Metadata["direction"] = TX
		nexthop.Metadata["inner_smac"] = Rmac.String()
		if Rmac == nil || len(Rmac) == 0 {
			nexthop.Resolved = false
		}
		vtepip := G.Spec.VtepIP.IP
		nexthop.Metadata["local_vtep_ip"] = vtepip.String()
		nexthop.Metadata["remote_vtep_ip"] = nexthop.nexthop.Gw.String()
		nexthop.Metadata["vni"] = *nexthop.Vrf.Spec.Vni
		if nexthop.Neighbor != nil {
			nexthop.Metadata["inner_dmac"] = nexthop.Neighbor.Neigh0.HardwareAddr.String()
			G, err := infradb.GetVrf("//network.opiproject.org/vrfs/GRD")
			if err == nil {
				r, ok := lookupRoute(nexthop.nexthop.Gw, G)
				if ok {
					// For now pick the first physical nexthop (no ECMP yet)
					phyNh := r.Nexthops[0]
					link, _ := vn.LinkByName(NameIndex[phyNh.nexthop.LinkIndex])
					nexthop.Metadata["phy_smac"] = link.Attrs().HardwareAddr.String()
					nexthop.Metadata["egress_vport"] = phyPorts[NameIndex[phyNh.nexthop.LinkIndex]]
					if phyNh.Neighbor != nil {
						nexthop.Metadata["phy_dmac"] = phyNh.Neighbor.Neigh0.HardwareAddr.String()
					} else {
						// The VXLAN nexthop can only be installed when the phy_nexthops are Resolved.
						nexthop.Resolved = false
					}
				}
			} else {
				log.Printf("netlink: No GRD found :%v\n", err)
			}
		} else {
			nexthop.Resolved = false
		}
	} else {
		nexthop.NhType = ACC
		link1, err := vn.LinkByName("rep-" + path.Base(nexthop.Vrf.Name))
		if err != nil {
			log.Printf("netlink: Error in getting rep information: %v\n", err)
		}
		if link1 == nil {
			return nexthop
		}
		nexthop.Metadata["direction"] = RX
		nexthop.Metadata["dmac"] = link1.Attrs().HardwareAddr.String()
		nexthop.Metadata["egress_vport"] = (int((link1.Attrs().HardwareAddr)[0]) << 8) + int((link1.Attrs().HardwareAddr)[1])
		if nexthop.Vrf.Spec.Vni == nil {
			nexthop.Metadata["vlanID"] = uint32(4089)
		} else {
			nexthop.Metadata["vlanID"] = *nexthop.Vrf.Metadata.RoutingTable[0] //*nexthop.Vrf.Spec.Vni
		}
	}
	return nexthop
}

// nolint
func neighborAnnotate(neighbor neighStruct) neighStruct {
	neighbor.Metadata = make(map[interface{}]interface{})
	var phyFlag bool
	phyFlag = false
	for k := range phyPorts {
		if NameIndex[neighbor.Neigh0.LinkIndex] == k {
			phyFlag = true
		}
	}
	if strings.HasPrefix(NameIndex[neighbor.Neigh0.LinkIndex], path.Base(neighbor.VrfName)) && neighbor.Protocol != zebraStr {
		pattern := fmt.Sprintf(`%s-\d+$`, path.Base(neighbor.VrfName))
		mustcompile := regexp.MustCompile(pattern)
		s := mustcompile.FindStringSubmatch(NameIndex[neighbor.Neigh0.LinkIndex])
		var LB *infradb.LogicalBridge
		var BP *infradb.BridgePort
		vID := strings.Split(s[0], "-")[1]
		lbs, _ := infradb.GetAllLBs()
		vlanID, err := strconv.ParseUint(vID, 10, 32)
		if err != nil {
			panic(err)
		}
		for _, lb := range lbs {
			if lb.Spec.VlanID == uint32(vlanID) {
				LB = lb
				break
			}
		}
		if LB != nil {
			bp := LB.MacTable[neighbor.Neigh0.HardwareAddr.String()]
			if bp != "" {
				BP, _ = infradb.GetBP(bp)
			}
		}
		if BP != nil {
			neighbor.Type = SVI
			neighbor.Metadata["vport_id"] = BP.Metadata.VPort
			neighbor.Metadata["vlanID"] = uint32(vlanID)
			neighbor.Metadata["portType"] = BP.Spec.Ptype
		} else {
			neighbor.Type = IGNORE
		}
	} else if strings.HasPrefix(NameIndex[neighbor.Neigh0.LinkIndex], path.Base(neighbor.VrfName)) && neighbor.Protocol == zebraStr {
		pattern := fmt.Sprintf(`%s-\d+$`, path.Base(neighbor.VrfName))
		mustcompile := regexp.MustCompile(pattern)
		s := mustcompile.FindStringSubmatch(neighbor.Dev)
		var LB *infradb.LogicalBridge
		vID := strings.Split(s[0], "-")[1]
		lbs, _ := infradb.GetAllLBs()
		vlanID, err := strconv.ParseUint(vID, 10, 32)
		if err != nil {
			panic(err)
		}
		for _, lb := range lbs {
			if lb.Spec.VlanID == uint32(vlanID) {
				LB = lb
				break
			}
		}
		if LB.Spec.Vni != nil {
			vid, err := strconv.Atoi(vID)
			if err != nil {
				panic(err)
			}
			fdbEntry := latestFDB[fdbKey{vid, neighbor.Neigh0.HardwareAddr.String()}]
			neighbor.Metadata["l2_nh"] = fdbEntry.Nexthop
			neighbor.Type = VXLAN // confirm this later
		}
	} else if path.Base(neighbor.VrfName) == "GRD" && phyFlag && neighbor.Protocol != zebraStr {
		VRF, _ := infradb.GetVrf("//network.opiproject.org/vrfs/GRD")
		r, ok := lookupRoute(neighbor.Neigh0.IP, VRF)
		if ok {
			if r.Nexthops[0].nexthop.LinkIndex == neighbor.Neigh0.LinkIndex {
				neighbor.Type = PHY
				neighbor.Metadata["vport_id"] = phyPorts[NameIndex[neighbor.Neigh0.LinkIndex]]
			} else {
				neighbor.Type = IGNORE
			}
		} else {
			neighbor.Type = OTHER
		}
	}
	return neighbor
}

// lookupRoute check the routes
func lookupRoute(dst net.IP, v *infradb.Vrf) (*RouteStruct, bool) {
	// FIXME: If the semantic is to return the current entry of the NetlinkDB
	//  routing table, a direct lookup in Linux should only be done as fallback
	//  if there is no match in the DB.
	var CP string
	var err error
	var RouteData []routeCmdInfo
	if v.Spec.Vni != nil {
		CP, err = nlink.RouteLookup(ctx, dst.String(), path.Base(v.Name))
	} else {
		CP, err = nlink.RouteLookup(ctx, dst.String(), "")
	}
	if err != nil || len(CP) <= 3 {
		log.Printf("netlink : Command error %v\n", err)
		return &RouteStruct{}, false
	}
	var rawMessages []json.RawMessage
	err = json.Unmarshal([]byte(CP), &rawMessages)
	if err != nil {
		log.Printf("JSON unmarshal error: %v", err)
		return &RouteStruct{}, false
	}
	CPs := make([]string, 0, len(rawMessages))
	for _, rawMsg := range rawMessages {
		CPs = append(CPs, string(rawMsg))
	}
	for i := 0; i < len(CPs); i++ {
		var ri routeCmdInfo
		err := json.Unmarshal([]byte(CPs[i]), &ri)
		if err != nil {
			log.Println("error-", err)
			return &RouteStruct{}, false
		}
		RouteData = append(RouteData, ri)
	}
	r := cmdProcessRt(v, RouteData, int(*v.Metadata.RoutingTable[0]))
	if len(r.RS) != 0 {
		R1 := r.RS[0]
		// ###  Search the latestRoutes DB snapshot if that exists, else
		// ###  the current DB Route table.
		var RouteTable map[routeKey]*RouteStruct
		if len(latestRoutes) != 0 {
			RouteTable = latestRoutes
		} else {
			RouteTable = routes
		}
		RDB, ok := RouteTable[R1.Key]
		if ok {
			// Return the existing route in the DB
			return RDB, ok
		}
		// Return the just constructed non-DB route
		return R1, true
	}

	log.Printf("netlink: Failed to lookup route %v in VRF %v", dst, v)
	return &RouteStruct{}, false
}
