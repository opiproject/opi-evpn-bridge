package netlink

import (
	"fmt"
	"log"
	"net"
	"reflect"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	vn "github.com/vishvananda/netlink"
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
func addL2Nexthop(m *FdbEntryStruct) *FdbEntryStruct {
	latestNexthops := latestL2Nexthop[m.Nexthop.Key]
	if !(reflect.ValueOf(latestNexthops).IsZero()) {
		latestNexthops.FdbRefs = append(latestNexthops.FdbRefs, m)
		m.Nexthop = latestNexthops
	} else {
		latestNexthops = m.Nexthop
		latestNexthops.FdbRefs = append(latestNexthops.FdbRefs, m)
		latestNexthops.ID = L2NHAssignID(latestNexthops.Key)
		latestL2Nexthop[latestNexthops.Key] = latestNexthops
		m.Nexthop = latestNexthops
	}
	return m
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
func installFilterL2N(l2n *L2NexthopStruct) bool {
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
