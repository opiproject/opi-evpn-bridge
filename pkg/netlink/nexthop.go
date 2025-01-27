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
	Weight  int
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
	Dir       int
	Divisor   int
	Value     float64
	Hashes    []int
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
	VRFNEIGHBOR
	SVI
	ACC
	VXLAN
	BRIDGEPORT
	OTHER
	IGNORE
	ECMP
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

// deepCopyMetadata deep copies the metadata
func deepCopyMetadata(originalMap map[interface{}]interface{}) map[interface{}]interface{} {
	newMap := make(map[interface{}]interface{})
	for key, value := range originalMap {
		newMap[key] = value
	}
	return newMap
}

// tryResolve resolves the neighbor
func (nexthop *NexthopStruct) tryResolve() []*NexthopStruct {
	var retNexthopSt []*NexthopStruct
	if nexthop.Metadata == nil {
		nexthop.Metadata = make(map[interface{}]interface{})
	}
	if nexthop.nexthop.Gw != nil {
		// Nexthops with a gateway IP need resolution of that IP
		neighborKey := NeighKey{Dst: nexthop.nexthop.Gw.String(), VrfName: nexthop.Vrf.Name, Dev: nexthop.nexthop.LinkIndex}
		ch := checkNeigh(neighborKey)
		if ch {
			if nexthop.NhType == VXLAN {
				nexthop.Metadata["remote_vtep_ip"] = nexthop.nexthop.Gw.String()
				nh := latestNeighbors[neighborKey]
				nexthop.Metadata["inner_dmac"] = nh.Neigh0.HardwareAddr.String()
				VRF, _ := infradb.GetVrf("//network.opiproject.org/vrfs/GRD")
				r, ok := lookupRoute(nexthop.nexthop.Gw, VRF)
				if ok {
					for _, grdNexthop := range r.Nexthops {
						arrayOfNexthops := grdNexthop.tryResolve()
						if len(arrayOfNexthops) != 0 {
							nexthopSt := *nexthop
							nexthopSt.nexthop.Gw = grdNexthop.nexthop.Gw
							nexthopSt.nexthop.LinkIndex = grdNexthop.nexthop.LinkIndex
							nexthopSt.Key = NexthopKey{nexthopSt.Vrf.Name, nexthopSt.nexthop.Gw.String(), nexthopSt.nexthop.LinkIndex, nexthopSt.Local, nexthopSt.Weight}
							nexthopSt.Neighbor = grdNexthop.Neighbor
							nexthopSt.Weight = grdNexthop.Weight
							nexthopSt.RouteRefs = nexthop.RouteRefs
							nexthopSt.Metadata = deepCopyMetadata(nexthop.Metadata)

							nexthopSt.Resolved = true
							retNexthopSt = append(retNexthopSt, &nexthopSt)
						}
					}
				}
				return retNexthopSt
			} else if nexthop.NhType >= 0 {
				nexthop.Resolved = true
				nh := latestNeighbors[neighborKey]
				nexthop.Neighbor = &nh
				return []*NexthopStruct{nexthop}
			}
			nexthop.Resolved = false
			nexthop.Neighbor = nil
			return nil
		}
		nexthop.Resolved = false
		nexthop.Neighbor = nil
		return nil
	}
	nexthop.Resolved = true
	return []*NexthopStruct{nexthop}
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
//
// nolint
func (nexthop *NexthopStruct) addNexthop(r *RouteStruct) *RouteStruct {
	if len(r.Nexthops) > 0 && !enableEcmp {
		log.Printf("ECMP disabled: Ignoring additional nexthop of route")
		return nil
	}
	ch := checkNhDB(nexthop.Key)
	if ch {
		NH0 := latestNexthop[nexthop.Key]
		// Links route with existing nexthop
		NH0.RouteRefs = append(NH0.RouteRefs, r)
		r.Nexthops = append(r.Nexthops, NH0)
	} else if nexthop.Resolved {
		nexthop.RouteRefs = append(nexthop.RouteRefs, r)
		nexthop.ID = NHAssignID(nexthop.Key)
		latestNexthop[nexthop.Key] = nexthop
		r.Nexthops = append(r.Nexthops, nexthop)
	} else {
		nexthops := nexthop.tryResolve()
		for _, nexthop := range nexthops {
			r = nexthop.addNexthop(r)
		}
	}
	return r
}

// ParseNexthop parses the neighbor
//
// nolint
func (nexthop *NexthopStruct) ParseNexthop(v *infradb.Vrf, rc RouteCmdInfo) {
	var phyFlag bool
	phyFlag = false

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

	for k := range phyPorts {
		if nameIndex[nexthop.nexthop.LinkIndex] == k {
			phyFlag = true
		}
	}
	if (nexthop.nexthop.Gw != nil && !nexthop.nexthop.Gw.IsUnspecified()) && phyFlag && !nexthop.Local {
		nexthop.NhType = PHY
	} else if (nexthop.nexthop.Gw != nil && !nexthop.nexthop.Gw.IsUnspecified()) && nexthop.nexthop.LinkIndex != 0 && strings.HasPrefix(nameIndex[nexthop.nexthop.LinkIndex], path.Base(nexthop.Vrf.Name)+"-") && !nexthop.Local {
		nexthop.NhType = VRFNEIGHBOR
	} else if (nexthop.nexthop.Gw != nil && !nexthop.nexthop.Gw.IsUnspecified()) && nameIndex[nexthop.nexthop.LinkIndex] == fmt.Sprintf("br-%s", path.Base(nexthop.Vrf.Name)) && !nexthop.Local {
		nexthop.NhType = VXLAN
	} else {
		nexthop.NhType = ACC
	}
	nexthop.Key = NexthopKey{nexthop.Vrf.Name, nexthop.nexthop.Gw.String(), nexthop.nexthop.LinkIndex, nexthop.Local, nexthop.Weight}
}

// nolint
func (nexthop *NexthopStruct) annotate() {
	if nexthop.NhType == VRFNEIGHBOR {
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
				log.Printf("netlink: Failed to gather data for nexthop on physical port with nexthop is %+v\n", nexthop)
			}
		}
	} else if nexthop.NhType == PHY {
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
			log.Printf("netlink: Failed to gather data for nexthop on physical port with nexthop is %+v\n", nexthop)
		}
	} else if nexthop.NhType == VXLAN {
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
		nexthop.Metadata["vni"] = *nexthop.Vrf.Spec.Vni
		if nexthop.Neighbor.Type == PHY {
			r, ok := lookupRoute(nexthop.nexthop.Gw, v)
			if ok {
				phyNh := r.Nexthops[0]
				link, _ := vn.LinkByName(nameIndex[phyNh.nexthop.LinkIndex])
				nexthop.Metadata["phy_smac"] = link.Attrs().HardwareAddr.String()
				nexthop.Metadata["egress_vport"] = phyPorts[nameIndex[phyNh.nexthop.LinkIndex]]
				nexthop.Metadata["phy_dmac"] = nexthop.Neighbor.Neigh0.HardwareAddr.String() // link.Attrs().HardwareAddr.String()
			}
		}
	} else if nexthop.NhType == ACC {
		//nexthop.NhType = ACC
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
	} else {
		nexthop.Resolved = false
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
func (nexthop *NexthopStruct) installFilterNH() bool {
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
	s = "Nexthop table:\n"
	for _, n := range nexthops {
		str := fmt.Sprintf("Nexthop(id=%d vrf=%s dst=%s dev=%s Local=%t weight=%d flags=[%s] #routes=%d Resolved=%t neighbor=%s) ", n.ID, n.Vrf.Name, n.nexthop.Gw.String(), nameIndex[n.nexthop.LinkIndex], n.Local, n.Weight, getFlagString(n.nexthop.Flags), len(n.RouteRefs), n.Resolved, n.Neighbor.printNeigh())
		s += str
		s += "\n"
	}
	s += "\n\n"
	return s
}
