package netlink

import vn "github.com/vishvananda/netlink"

// nolint
func applyInstallFilters() {
	for K, r := range latestRoutes {
		if !installFilterRoute(r) {
			// Remove route from its nexthop(s)
			delete(latestRoutes, K)
		}
	}

	for k, nexthop := range latestNexthop {
		if !installFilterNH(nexthop) {
			delete(latestNexthop, k)
		}
	}

	for k, m := range latestFDB {
		if !installFilterFDB(m) {
			delete(latestFDB, k)
		}
	}
	for k, L2 := range latestL2Nexthop {
		if !installFilterL2N(L2) {
			delete(latestL2Nexthop, k)
		}
	}
}

// installFilterL2N install the l2 filter
func installFilterL2N(l2n *L2NexthopStruct) bool {
	keep := !(l2n.Type == 0 && l2n.Resolved && len(l2n.FdbRefs) == 0)
	return keep
}

// installFilterFDB install fdb filer
func installFilterFDB(fdb *FdbEntryStruct) bool {
	// Drop entries w/o VLAN ID or associated LogicalBridge ...
	// ... other than with L2 nexthops of type VXLAN and BridgePort ...
	// ... and VXLAN entries with unresolved underlay nextop.
	keep := fdb.VlanID != 0 && fdb.lb != nil && checkFdbType(fdb.Type) && fdb.Nexthop.Resolved
	return keep
}

// installFilterNH install the neighbor filter
func installFilterNH(nh *NexthopStruct) bool {
	check := checkNhType(nh.NhType)
	keep := check && nh.Resolved && len(nh.RouteRefs) != 0
	return keep
}

// installFilterRoute install the route filter
func installFilterRoute(routeSt *RouteStruct) bool {
	var nh []*NexthopStruct
	for _, n := range routeSt.Nexthops {
		if n.Resolved {
			nh = append(nh, n)
		}
	}
	routeSt.Nexthops = nh
	keep := checkRtype(routeSt.NlType) && len(nh) != 0 && routeSt.Route0.Dst.IP.String() != "0.0.0.0"
	return keep
}

func checkFdbType(fdbtype int) bool {
	var portType = []int{BRIDGEPORT, VXLAN}
	for _, port := range portType {
		if port == fdbtype {
			return true
		}
	}
	return false
}

// checkNhType checks the nighbor type
func checkNhType(nType int) bool {
	ntype := []int{PHY, SVI, ACC, VXLAN}
	for _, i := range ntype {
		if i == nType {
			return true
		}
	}
	return false
}

// checkRtype checks the route type
func checkRtype(rType string) bool {
	var Types = [6]string{routeTypeConnected, routeTypeEvpnVxlan, routeTypeStatic, routeTypeBgp, routeTypeLocal, routeTypeNeighbor}
	for _, v := range Types {
		if v == rType {
			return true
		}
	}
	return false
}

// preFilterMac filter the mac
func preFilterMac(f *FdbEntryStruct) bool {
	// TODO m.nexthop.dst
	if f.VlanID != 0 || (f.Nexthop.Dst != nil && !f.Nexthop.Dst.IsUnspecified()) {
		return true
	}
	return false
}

// preFilterRoute pre filter the routes
func preFilterRoute(r *RouteStruct) bool {
	if checkRtype(r.NlType) && !r.Route0.Dst.IP.IsLoopback() && r.Route0.Dst.IP.String() != "0.0.0.0" {
		return true
	}

	return false
}

// preFilterNeighbor pre filter the neighbors
func preFilterNeighbor(n neighStruct) bool {
	if n.Neigh0.State != vn.NUD_NONE && n.Neigh0.State != vn.NUD_INCOMPLETE && n.Neigh0.State != vn.NUD_FAILED && NameIndex[n.Neigh0.LinkIndex] != "lo" {
		return true
	}

	return false
}
