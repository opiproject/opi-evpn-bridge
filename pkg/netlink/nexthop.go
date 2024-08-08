package netlink

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"path"
	"reflect"
	"strings"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	vn "github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// NexthopKey structure of nexthop
type NexthopKey struct {
	VrfName string
	Dst     string
	Dev     int
	Local   bool
}

// NexthopStruct contains nexthop structure
type NexthopStruct struct {
	nexthop   vn.NexthopInfo
	Vrf       *infradb.Vrf
	Local     bool
	Weight    int
	Metric    int
	ID        int
	Scope     int
	Protocol  int
	RouteRefs []*RouteStruct
	Key       NexthopKey
	Resolved  bool
	Neighbor  *NeighStruct
	NhType    int
	Metadata  map[interface{}]interface{}
}

// nexthopOperations add, update, delete
var nexthopOperations = Operations{Add: NexthopAdded, Update: NexthopUpdated, Delete: NexthopDeleted}

// nhNextID Variable
var nhNextID = 16

// Nexthops Variable
var nexthops = make(map[NexthopKey]*NexthopStruct)

// latestNexthop Variable
var latestNexthop = make(map[NexthopKey]*NexthopStruct)

// Event Operations
const (
	// NexthopAdded event const
	NexthopAdded = "nexthop_added"
	// NexthopUpdated event const
	NexthopUpdated = "nexthop_updated"
	// NexthopDeleted event const
	NexthopDeleted = "nexthop_deleted"
)

// Nexthop type
const ( // NexthopStruct TYPE & L2NEXTHOP TYPE & FDBentry
	PHY = iota
	SVI
	ACC
	VXLAN
	BRIDGEPORT
	OTHER
	IGNORE
)

// checkNhDB checks the neighbor database
func checkNhDB(nhKey NexthopKey) bool {
	for k := range latestNexthop {
		if k == nhKey {
			return true
		}
	}
	return false
}

// tryResolve resolves the neighbor
func (nexthop *NexthopStruct) tryResolve() *NexthopStruct {
	if nexthop.nexthop.Gw != nil {
		// Nexthops with a gateway IP need resolution of that IP
		neighborKey := NeighKey{Dst: nexthop.nexthop.Gw.String(), VrfName: nexthop.Vrf.Name, Dev: nexthop.nexthop.LinkIndex}
		ch := checkNeigh(neighborKey)
		if ch && latestNeighbors[neighborKey].Type != IGNORE {
			nexthop.Resolved = true
			nh := latestNeighbors[neighborKey]
			nexthop.Neighbor = &nh
		} else {
			nexthop.Resolved = false
		}
	} else {
		nexthop.Resolved = true
	}
	return nexthop
}

// NHAssignID returns the nexthop id
func NHAssignID(key NexthopKey) int {
	id := nhIDCache[key]
	if id == 0 {
		// Assigne a free id and insert it into the cache
		id = nhNextID
		nhIDCache[key] = id
		nhNextID++
	}
	return id
}

// addNexthop adds the nexthop
func (nexthop *NexthopStruct) addNexthop(r *RouteStruct) *RouteStruct {
	ch := checkNhDB(nexthop.Key)
	if ch {
		nh0 := latestNexthop[nexthop.Key]
		// Links route with existing nexthop
		nh0.RouteRefs = append(nh0.RouteRefs, r)
		r.Nexthops = append(r.Nexthops, nh0)
	} else {
		// Create a new nexthop entry
		nexthop.RouteRefs = append(nexthop.RouteRefs, r)
		nexthop.ID = NHAssignID(nexthop.Key)
		nexthop = nexthop.tryResolve()
		latestNexthop[nexthop.Key] = nexthop
		r.Nexthops = append(r.Nexthops, nexthop)
	}
	return r
}

// ParseNexthop parses the neighbor
func (nexthop *NexthopStruct) ParseNexthop(v *infradb.Vrf, rc RouteCmdInfo) {
	nexthop.Weight = 1
	nexthop.Vrf = v
	if rc.Dev != "" {
		vrf, _ := vn.LinkByName(rc.Dev)
		nexthop.nexthop.LinkIndex = vrf.Attrs().Index
		nameIndex[nexthop.nexthop.LinkIndex] = vrf.Attrs().Name
	}
	if len(rc.Flags) != 0 {
		nexthop.nexthop.Flags = getFlag(rc.Flags[0])
	}
	if rc.Gateway != "" {
		nIP := &net.IPNet{
			IP: net.ParseIP(rc.Gateway),
		}
		nexthop.nexthop.Gw = nIP.IP
	}
	if rc.Protocol != "" {
		nexthop.Protocol = rtnProto[rc.Protocol]
	}
	if rc.Scope != "" {
		nexthop.Scope = rtnScope[rc.Scope]
	}
	if rc.Type != "" {
		nexthop.NhType = rtnType[rc.Type]
		if nexthop.NhType == unix.RTN_LOCAL {
			nexthop.Local = true
		} else {
			nexthop.Local = false
		}
	}
	if rc.Weight >= 0 {
		nexthop.Weight = rc.Weight
	}
	nexthop.Key = NexthopKey{nexthop.Vrf.Name, nexthop.nexthop.Gw.String(), nexthop.nexthop.LinkIndex, nexthop.Local}
}

// nolint
func (nexthop *NexthopStruct) annotate() {
	nexthop.Metadata = make(map[interface{}]interface{})
	var phyFlag bool
	phyFlag = false
	for k := range phyPorts {
		if nameIndex[nexthop.nexthop.LinkIndex] == k {
			phyFlag = true
		}
	}
	if (nexthop.nexthop.Gw != nil && !nexthop.nexthop.Gw.IsUnspecified()) && nexthop.nexthop.LinkIndex != 0 && strings.HasPrefix(nameIndex[nexthop.nexthop.LinkIndex], path.Base(nexthop.Vrf.Name)+"-") && !nexthop.Local {
		nexthop.NhType = SVI
		link, _ := vn.LinkByName(nameIndex[nexthop.nexthop.LinkIndex])
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
				L2N, ok := nexthop.Neighbor.Metadata["l2_nh"].(L2NexthopStruct)
				if !ok {
					log.Printf("netlink: Neighbor metadata l2_nh is not of L2NexthopStruct type")
					return
				}
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
		link1, _ := vn.LinkByName(nameIndex[nexthop.nexthop.LinkIndex])
		if link1 == nil {
			return
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
	} else if (nexthop.nexthop.Gw != nil && !nexthop.nexthop.Gw.IsUnspecified()) && nameIndex[nexthop.nexthop.LinkIndex] == fmt.Sprintf("br-%s", path.Base(nexthop.Vrf.Name)) && !nexthop.Local {
		nexthop.NhType = VXLAN
		v, _ := infradb.GetVrf(nexthop.Vrf.Name)
		var detail map[string]interface{}
		var rmac net.HardwareAddr
		for _, com := range v.Status.Components {
			if com.Name == "frr" {
				err := json.Unmarshal([]byte(com.Details), &detail)
				if err != nil {
					log.Printf("netlink nexthop: Error: %v %v : %v", err, com.Details, detail)
					break
				}
				mac, ok := detail["rmac"]
				if !ok {
					log.Printf("netlink: Key 'rmac' not found")
					break
				}
				strRmac, found := mac.(string)
				if !found || strRmac == "" {
					log.Printf("netlink: key 'rmac' is empty")
					break
				}
				rmac, err = net.ParseMAC(strRmac)
				if err != nil {
					log.Printf("netlink: Error parsing MAC address: %v", err)
				}
			}
		}
		nexthop.Metadata["direction"] = TX
		nexthop.Metadata["inner_smac"] = rmac.String()
		if len(rmac) == 0 {
			nexthop.Resolved = false
		}
		vtepip := v.Spec.VtepIP.IP
		nexthop.Metadata["local_vtep_ip"] = vtepip.String()
		nexthop.Metadata["remote_vtep_ip"] = nexthop.nexthop.Gw.String()
		nexthop.Metadata["vni"] = *nexthop.Vrf.Spec.Vni
		if nexthop.Neighbor != nil {
			nexthop.Metadata["inner_dmac"] = nexthop.Neighbor.Neigh0.HardwareAddr.String()
			v, err := infradb.GetVrf("//network.opiproject.org/vrfs/GRD")
			if err == nil {
				r, ok := lookupRoute(nexthop.nexthop.Gw, v)
				if ok {
					// For now pick the first physical nexthop (no ECMP yet)
					phyNh := r.Nexthops[0]
					link, _ := vn.LinkByName(nameIndex[phyNh.nexthop.LinkIndex])
					nexthop.Metadata["phy_smac"] = link.Attrs().HardwareAddr.String()
					nexthop.Metadata["egress_vport"] = phyPorts[nameIndex[phyNh.nexthop.LinkIndex]]
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
			return
		}
		nexthop.Metadata["direction"] = RX
		nexthop.Metadata["dmac"] = link1.Attrs().HardwareAddr.String()
		nexthop.Metadata["egress_vport"] = (int((link1.Attrs().HardwareAddr)[0]) << 8) + int((link1.Attrs().HardwareAddr)[1])
		if nexthop.Vrf.Spec.Vni == nil {
			nexthop.Metadata["vlanID"] = uint32(4089)
		} else {
			nexthop.Metadata["vlanID"] = *nexthop.Vrf.Metadata.RoutingTable[0]
		}
	}
}

// checkNhType checks the nighbor type
func checkNhType(nType int) bool {
	ntype := map[int]struct{}{PHY: {}, SVI: {}, ACC: {}, VXLAN: {}}
	if _, ok := ntype[nType]; ok {
		return true
	}
	return false
}

// installFilterNH install the neighbor filter
func (nexthop *NexthopStruct) filter() bool {
	check := checkNhType(nexthop.NhType)
	keep := check && nexthop.Resolved && len(nexthop.RouteRefs) != 0
	return keep
}

func (nexthop *NexthopStruct) deepEqual(nhOld *NexthopStruct, nc bool) bool {
	if nexthop.Vrf.Name != nhOld.Vrf.Name || nexthop.Weight != nhOld.Weight || nexthop.ID != nhOld.ID || nexthop.Key != nhOld.Key || nexthop.Local != nhOld.Local ||
		!reflect.DeepEqual(nexthop.Metadata, nhOld.Metadata) || nexthop.Metric != nhOld.Metric ||
		nexthop.Scope != nhOld.Scope || nexthop.Resolved != nhOld.Resolved || nexthop.Protocol != nhOld.Protocol || nexthop.NhType != nhOld.NhType ||
		!reflect.DeepEqual(nexthop.nexthop, nhOld.nexthop) {
		return false
	}
	if nc {
		if len(nexthop.RouteRefs) != len(nhOld.RouteRefs) {
			return false
		}
		for i := range nexthop.RouteRefs {
			ret := nexthop.RouteRefs[i].deepEqual(nhOld.RouteRefs[i], false)
			if !ret {
				return false
			}
		}
	}
	return true
}

// GetVrfOperStatus gets nexthop vrf opration status
func (nexthop *NexthopStruct) GetVrfOperStatus() infradb.VrfOperStatus {
	return nexthop.Vrf.Status.VrfOperStatus
}

// dumpNexthDB dump the nexthop entries
func dumpNexthDB() string {
	var s string
	log.Printf("netlink: Dump Nexthop table:\n")
	s = "Nexthop table:\n"
	for _, n := range latestNexthop {
		str := fmt.Sprintf("Nexthop(id=%d vrf=%s dst=%s dev=%s Local=%t weight=%d flags=[%s] #routes=%d Resolved=%t neighbor=%s) ", n.ID, n.Vrf.Name, n.nexthop.Gw.String(), nameIndex[n.nexthop.LinkIndex], n.Local, n.Weight, getFlagString(n.nexthop.Flags), len(n.RouteRefs), n.Resolved, n.Neighbor.printNeigh())
		log.Println(str)
		s += str
		s += "\n"
	}
	log.Printf("\n\n\n")
	s += "\n\n"
	return s
}
