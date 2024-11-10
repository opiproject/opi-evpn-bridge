package netlink

import (
	"fmt"
	"log"
	"net"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	vn "github.com/vishvananda/netlink"
)

// L2NexthopKey is l2 neighbor key
type L2NexthopKey struct {
	Dev    string
	VlanID int
	Dst    string
}

// L2NexthopStruct structure
type L2NexthopStruct struct {
	Dev      string
	VlanID   int
	Dst      net.IP
	Key      L2NexthopKey
	lb       *infradb.LogicalBridge
	bp       *infradb.BridgePort
	ID       int
	FdbRefs  []*FdbEntryStruct
	Resolved bool
	Type     int
	Metadata map[interface{}]interface{}
}

// l2nexthopID
var l2nexthopID = 16

// l2Nexthops Variable
var l2Nexthops = make(map[L2NexthopKey]*L2NexthopStruct)

// latestL2Nexthop Variable
var latestL2Nexthop = make(map[L2NexthopKey]*L2NexthopStruct)

// l2NhIDCache
var l2NhIDCache = make(map[L2NexthopKey]int)

// l2NexthopOperations add, update, delete
var l2NexthopOperations = Operations{Add: L2NexthopAdded, Update: L2NexthopUpdated, Delete: L2NexthopDeleted}

// Event Operations
const (
	// L2NexthopAdded event const
	L2NexthopAdded = "l2_nexthop_added"
	// L2NexthopUpdated event const
	L2NexthopUpdated = "l2_nexthop_updated"
	// L2NexthopDeleted event const
	L2NexthopDeleted = "l2_nexthop_deleted"
)

// nolint
// ParseL2NH parse the l2hn
func (l2n *L2NexthopStruct) ParseL2NH(vlanID int, dev string, dst string, lb *infradb.LogicalBridge, bp *infradb.BridgePort) {
	l2n.Dev = dev
	l2n.VlanID = vlanID
	l2n.Dst = net.IP(dst)
	l2n.Key = L2NexthopKey{l2n.Dev, l2n.VlanID, string(l2n.Dst)}
	l2n.lb = lb
	l2n.bp = bp
	l2n.Resolved = true
	if l2n.Dev == fmt.Sprintf("svi-%d", l2n.VlanID) {
		l2n.Type = SVI
	} else if l2n.Dev == fmt.Sprintf("vxlan-%d", l2n.VlanID) {
		l2n.Type = VXLAN
	} else if l2n.bp != nil {
		l2n.Type = BRIDGEPORT
	} else {
		l2n.Type = None
	}
}

// L2NHAssignID get nexthop id
func L2NHAssignID(key L2NexthopKey) int {
	id := l2NhIDCache[key]
	if id == 0 {
		// Assigne a free id and insert it into the cache
		id = l2nexthopID
		l2NhIDCache[key] = id
		l2nexthopID++
	}
	return id
}

// addL2Nexthop add the l2 nexthop
func (fdb *FdbEntryStruct) addL2Nexthop() {
	latestNexthop, ok := latestL2Nexthop[fdb.Nexthop.Key]
	if ok && latestNexthop != nil {
		latestNexthop.FdbRefs = append(latestNexthop.FdbRefs, fdb)
		fdb.Nexthop = latestNexthop
	} else {
		latestNexthop = fdb.Nexthop
		latestNexthop.FdbRefs = append(latestNexthop.FdbRefs, fdb)
		latestNexthop.ID = L2NHAssignID(latestNexthop.Key)
		latestL2Nexthop[latestNexthop.Key] = latestNexthop
		fdb.Nexthop = latestNexthop
	}
}

// nolint
func (l2n *L2NexthopStruct) annotate() {
	// Annotate certain L2 Nexthops with additional information from LB and GRD
	l2n.Metadata = make(map[interface{}]interface{})
	lb := l2n.lb
	if lb != nil {
		if l2n.Type == SVI {
			l2n.Metadata["vrf_id"] = *lb.Spec.Vni
		} else if l2n.Type == VXLAN {
			//# Remote EVPN MAC address learned on the VXLAN interface
			//# The L2 nexthop must have a destination IP address in dst
			l2n.Resolved = false
			l2n.Metadata["local_vtep_ip"] = *lb.Spec.VtepIP
			l2n.Metadata["remote_vtep_ip"] = l2n.Dst
			l2n.Metadata["vni"] = *lb.Spec.Vni
			//# The below physical nexthops are needed to transmit the VXLAN-encapsuleted packets
			//# directly from the nexthop table to a physical port (and avoid another recirculation
			//# for route lookup in the GRD table.)
			vrf, _ := infradb.GetVrf("//network.opiproject.org/vrfs/GRD")
			r, ok := lookupRoute(l2n.Dst, vrf)
			if ok {
				//  # For now pick the first physical nexthop (no ECMP yet)
				phyNh := r.Nexthops[0]
				link, _ := vn.LinkByName(nameIndex[phyNh.nexthop.LinkIndex])
				l2n.Metadata["phy_smac"] = link.Attrs().HardwareAddr.String()
				l2n.Metadata["egress_vport"] = phyPorts[nameIndex[phyNh.nexthop.LinkIndex]]
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
}

// installFilterL2N install the l2 filter
func (l2n *L2NexthopStruct) installFilterL2N() bool {
	keep := !(l2n.Type == 0 && l2n.Resolved && len(l2n.FdbRefs) == 0)
	return keep
}

func (l2n *L2NexthopStruct) deepEqual(l2nOld *L2NexthopStruct, nc bool) bool {
	if l2n.Dev != l2nOld.Dev || !l2n.Dst.Equal(l2nOld.Dst) || l2n.ID != l2nOld.ID || l2n.Key != l2nOld.Key || l2n.Type != l2nOld.Type {
		return false
	}
	if nc {
		if len(l2n.FdbRefs) != len(l2nOld.FdbRefs) {
			return false
		}
		for i := range l2n.FdbRefs {
			ret := l2n.FdbRefs[i].deepEqual(l2nOld.FdbRefs[i], false)
			if !ret {
				return false
			}
		}
	}
	return true
}

// dumpL2NexthDB dump the l2 nexthop entries
func dumpL2NexthDB() string {
	var s string
	s = "L2 Nexthop table:\n"
	var ip string
	for _, n := range l2Nexthops {
		if n.Dst == nil {
			ip = strNone
		} else {
			ip = n.Dst.String()
		}
		str := fmt.Sprintf("L2Nexthop(id=%d dev=%s vlan=%d dst=%s type=%d #fDB entries=%d Resolved=%t) ", n.ID, n.Dev, n.VlanID, ip, n.Type, len(n.FdbRefs), n.Resolved)
		s += str
		s += "\n"
	}
	s += "\n\n"
	return s
}
