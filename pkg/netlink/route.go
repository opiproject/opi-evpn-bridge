package netlink

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"net"
	"path"
	"reflect"
	"strconv"
	"strings"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	vn "github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// readRoutes reads the routes
func readRoutes(v *infradb.Vrf) {
	readRouteFromIP(v)
}

// checkRoute checks the route
func checkRoute(r *RouteStruct) bool {
	rk := r.Key
	for k := range latestRoutes {
		if k == rk {
			return true
		}
	}
	return false
}

// nolint
func setRouteType(rs *RouteStruct, v *infradb.Vrf) string {
	if rs.Route0.Type == unix.RTN_UNICAST && rs.Route0.Protocol == unix.RTPROT_KERNEL && rs.Route0.Scope == unix.RT_SCOPE_LINK && len(rs.Nexthops) == 1 {
		// Connected routes are proto=kernel and scope=link with a netdev as single nexthop
		return routeTypeConnected
	} else if rs.Route0.Type == unix.RTN_UNICAST && int(rs.Route0.Protocol) == int('B') && rs.Route0.Scope == unix.RT_SCOPE_UNIVERSE {
		// EVPN routes to remote destinations are proto=bgp, scope global withipu_infra_mgr_db
		// all Nexthops residing on the br-<VRF name> bridge interface of the VRF.
		var devs []string
		if len(rs.Nexthops) != 0 {
			for _, d := range rs.Nexthops {
				devs = append(devs, nameIndex[d.nexthop.LinkIndex])
			}
			if len(devs) == 1 && devs[0] == "br-"+v.Name {
				return routeTypeEvpnVxlan
			}
			return routeTypeBgp
		}
	} else if rs.Route0.Type == unix.RTN_UNICAST && checkProto(int(rs.Route0.Protocol)) && rs.Route0.Scope == unix.RT_SCOPE_UNIVERSE {
		return routeTypeStatic
	} else if rs.Route0.Type == unix.RTN_LOCAL {
		return routeTypeLocal
	} else if rs.Route0.Type == rtNNeighbor {
		// Special /32 or /128 routes for Resolved neighbors on connected subnets
		return routeTypeNeighbor
	}
	return "unknown"
}

// addRoute add the route
func addRoute(r *RouteStruct) {
	ch := checkRoute(r)
	if ch {
		r0 := latestRoutes[r.Key]
		if r.Route0.Priority >= r0.Route0.Priority {
			// Route with lower metric exists and takes precedence
			log.Printf("netlink: Ignoring %+v  with higher metric than %+v\n", r, r0)
		} else {
			log.Printf("netlink: conflicts %+v with higher metric %+v. Will ignore it", r, r0)
		}
	} else {
		nexthops := r.Nexthops
		r.Nexthops = deleteNH(r.Nexthops)
		for _, nexthop := range nexthops {
			r = addNexthop(nexthop, r)
		}
		latestRoutes[r.Key] = r
	}
}

// getNeighborRoutes gets the nighbor routes
func getNeighborRoutes() []RouteCmdInfo {
	// Return a list of /32 or /128 routes & Nexthops to be inserted into
	// the routing tables for Resolved neighbors on connected subnets
	// on physical and SVI interfaces.
	var neighborRoutes []RouteCmdInfo
	for _, n := range latestNeighbors {
		if n.Type == PHY || n.Type == SVI || n.Type == VXLAN {
			vrf, _ := infradb.GetVrf(n.VrfName)
			table := int(*vrf.Metadata.RoutingTable[0])

			//# Create a special route with dst == gateway to resolve
			//# the nexthop to the existing neighbor
			n0 := RouteCmdInfo{Type: routeTypeNeighbor, Dst: n.Neigh0.IP.String(), Protocol: "ipu_infra_mgr", Scope: "global", Gateway: n.Neigh0.IP.String(), Dev: nameIndex[n.Neigh0.LinkIndex], VRF: vrf, Table: table}
			neighborRoutes = append(neighborRoutes, n0)
		}
	}
	return neighborRoutes
}

// ParseRoute parse the routes
// nolint
func ParseRoute(v *infradb.Vrf, rc []RouteCmdInfo, t int) RouteList {
	var route RouteList
	for _, r0 := range rc {
		if r0.Type == "" && (r0.Dev != "" || r0.Gateway != "") {
			r0.Type = routeTypeLocal
		}
		var rs RouteStruct
		rs.Vrf = v
		if r0.Nhid != 0 || r0.Gateway != "" || r0.Dev != "" {
			rs.Nexthops = append(rs.Nexthops, NHParse(v, r0))
		}
		rs.NlType = "unknown"
		rs.Route0.Table = t
		rs.Route0.Priority = 1
		if r0.Dev != "" {
			dev, _ := vn.LinkByName(r0.Dev)
			rs.Route0.LinkIndex = dev.Attrs().Index
		}
		if r0.Dst != "" {
			var Mask int
			split := r0.Dst
			if strings.Contains(r0.Dst, "/") {
				split4 := strings.Split(r0.Dst, "/")
				Mask, _ = strconv.Atoi(split4[1])
				split = split4[0]
			} else {
				Mask = 32
			}
			var nIP *net.IPNet
			if r0.Dst == "default" {
				nIP = &net.IPNet{
					IP:   net.ParseIP("0.0.0.0"),
					Mask: net.IPv4Mask(0, 0, 0, 0),
				}
			} else {
				mtoip := netMaskToInt(Mask)
				b3 := make([]byte, 8) // Converting int64 to byte
				binary.LittleEndian.PutUint64(b3, uint64(mtoip[3]))
				b2 := make([]byte, 8)
				binary.LittleEndian.PutUint64(b2, uint64(mtoip[2]))
				b1 := make([]byte, 8)
				binary.LittleEndian.PutUint64(b1, uint64(mtoip[1]))
				b0 := make([]byte, 8)
				binary.LittleEndian.PutUint64(b0, uint64(mtoip[0]))
				nIP = &net.IPNet{
					IP:   net.ParseIP(split),
					Mask: net.IPv4Mask(b0[0], b1[0], b2[0], b3[0]),
				}
			}
			rs.Route0.Dst = nIP
		}
		if r0.Metric != 0 {
			rs.Route0.Priority = r0.Metric
		}
		if r0.Protocol != "" {
			if rtnProto[r0.Protocol] != 0 {
				rs.Route0.Protocol = vn.RouteProtocol(rtnProto[r0.Protocol])
			} else {
				rs.Route0.Protocol = 0
			}
		}
		if r0.Type != "" {
			rs.Route0.Type = rtnType[r0.Type]
		}
		if len(r0.Flags) != 0 {
			rs.Route0.Flags = getFlag(r0.Flags[0])
		}
		if r0.Scope != "" {
			rs.Route0.Scope = vn.Scope(rtnScope[r0.Scope])
		}
		if r0.Prefsrc != "" {
			nIP := &net.IPNet{
				IP: net.ParseIP(r0.Prefsrc),
			}
			rs.Route0.Src = nIP.IP
		}
		if r0.Gateway != "" {
			nIP := &net.IPNet{
				IP: net.ParseIP(r0.Gateway),
			}
			rs.Route0.Gw = nIP.IP
		}
		if r0.VRF != nil {
			rs.Vrf, _ = infradb.GetVrf(r0.VRF.Name)
		}
		if r0.Table != 0 {
			rs.Route0.Table = r0.Table
		}
		rs.NlType = setRouteType(&rs, v)
		rs.Key = RouteKey{Table: rs.Route0.Table, Dst: rs.Route0.Dst.String()}
		if preFilterRoute(&rs) {
			route.RS = append(route.RS, &rs)
		}
	}
	return route
}

// cmdProcessRt process the route command
func cmdProcessRt(v *infradb.Vrf, routeData []RouteCmdInfo, t int) RouteList {
	route := ParseRoute(v, routeData, t)
	return route
}

// readRouteFromIP reads the routes from ip
func readRouteFromIP(v *infradb.Vrf) {
	var rl RouteList
	var rm []RouteCmdInfo
	var rt int
	var routeData []RouteCmdInfo
	for _, routeSt := range v.Metadata.RoutingTable {
		rt = int(*routeSt)
		Raw, err := nlink.ReadRoute(ctx, strconv.Itoa(rt))
		if err != nil || len(Raw) <= 3 {
			log.Printf("netlink: Err Command route\n")
			continue
		}
		var rawMessages []json.RawMessage
		err = json.Unmarshal([]byte(Raw), &rawMessages)
		if err != nil {
			log.Printf("netlink route: JSON unmarshal error: %v %v : %v", err, Raw, rawMessages)
			continue
		}
		CPs := make([]string, 0, len(rawMessages))
		for _, rawMsg := range rawMessages {
			CPs = append(CPs, string(rawMsg))
		}
		for i := 0; i < len(CPs); i++ {
			var ri RouteCmdInfo
			err := json.Unmarshal([]byte(CPs[i]), &ri)
			if err != nil {
				log.Println("error-", err)
				continue
			}
			routeData = append(routeData, ri)
		}
		rl = cmdProcessRt(v, routeData, rt)
		for _, r := range rl.RS {
			addRoute(r)
		}
	}
	nl := getNeighborRoutes() // Add extra routes for Resolved neighbors on connected subnets
	for i := 0; i < len(nl); i++ {
		rm = append(rm, nl[i])
	}
	nr := ParseRoute(v, rm, 0)
	for _, r := range nr.RS {
		addRoute(r)
	}
}

// getProto gets the route protocol
func getProto(n *RouteStruct) string {
	for p, i := range rtnProto {
		if i == int(n.Route0.Protocol) {
			return p
		}
	}
	return "0"
}

// CheckRdup checks the duplication of routes
func CheckRdup(tmpKey RouteKey) bool {
	var dup = false
	for j := range latestRoutes {
		if j == tmpKey {
			dup = true
			break
		}
	}
	return dup
}

// annotate function annonates the entries
func (route *RouteStruct) annotate() {
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
}

// lookupRoute check the routes
func lookupRoute(dst net.IP, v *infradb.Vrf) (*RouteStruct, bool) {
	// FIXME: If the semantic is to return the current entry of the NetlinkDB
	//  routing table, a direct lookup in Linux should only be done as fallback
	//  if there is no match in the DB.
	var cp string
	var err error
	var routeData []RouteCmdInfo
	if v.Spec.Vni != nil {
		cp, err = nlink.RouteLookup(ctx, dst.String(), path.Base(v.Name))
	} else {
		cp, err = nlink.RouteLookup(ctx, dst.String(), "")
	}
	if err != nil || len(cp) <= 3 {
		log.Printf("netlink : Command error %v\n", err)
		return &RouteStruct{}, false
	}
	var rawMessages []json.RawMessage
	err = json.Unmarshal([]byte(cp), &rawMessages)
	if err != nil {
		log.Printf("netlink route: JSON unmarshal error: %v %v : %v\n", err, cp, rawMessages)
		return &RouteStruct{}, false
	}
	cps := make([]string, 0, len(rawMessages))
	for _, rawMsg := range rawMessages {
		cps = append(cps, string(rawMsg))
	}
	for i := 0; i < len(cps); i++ {
		var ri RouteCmdInfo
		err := json.Unmarshal([]byte(cps[i]), &ri)
		if err != nil {
			log.Println("error-", err)
			return &RouteStruct{}, false
		}
		routeData = append(routeData, ri)
	}
	r := cmdProcessRt(v, routeData, int(*v.Metadata.RoutingTable[0]))
	if len(r.RS) != 0 {
		r0 := r.RS[0]
		// ###  Search the latestRoutes DB snapshot if that exists, else
		// ###  the current DB Route table.
		var RouteTable map[RouteKey]*RouteStruct
		if len(latestRoutes) != 0 {
			RouteTable = latestRoutes
		} else {
			RouteTable = routes
		}
		RDB, ok := RouteTable[r0.Key]
		if ok {
			// Return the existing route in the DB
			return RDB, ok
		}
		// Return the just constructed non-DB route
		return r0, true
	}

	log.Printf("netlink: Failed to lookup route %v in VRF %v", dst, v)
	return &RouteStruct{}, false
}

// checkRtype checks the route type
func checkRtype(rType string) bool {
	var Types = map[string]struct{}{routeTypeConnected: {}, routeTypeEvpnVxlan: {}, routeTypeStatic: {}, routeTypeBgp: {}, routeTypeLocal: {}, routeTypeNeighbor: {}}
	if _, ok := Types[rType]; ok {
		return true
	}
	return false
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

// preFilterRoute pre filter the routes
func preFilterRoute(r *RouteStruct) bool {
	if checkRtype(r.NlType) && !r.Route0.Dst.IP.IsLoopback() && r.Route0.Dst.IP.String() != "0.0.0.0" {
		return true
	}

	return false
}

func (route *RouteStruct) deepEqual(routeOld *RouteStruct, nc bool) bool {
	if route.Vrf.Name != routeOld.Vrf.Name || !reflect.DeepEqual(route.Route0, routeOld.Route0) || route.Key != routeOld.Key ||
		route.NlType != routeOld.NlType || !reflect.DeepEqual(route.Metadata, routeOld.Metadata) {
		return false
	}
	if nc {
		if len(route.Nexthops) != len(routeOld.Nexthops) {
			return false
		}
		return route.Nexthops[0].deepEqual(routeOld.Nexthops[0], false)
	}
	return true
}

// GetVrfOperStatus gets route vrf operation status
func (route *RouteStruct) GetVrfOperStatus() infradb.VrfOperStatus {
	return route.Vrf.Status.VrfOperStatus
}
