package netlink

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
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

// routes Variable
var routes = make(map[RouteKey]*RouteStruct)

// latestRoutes Variable
var latestRoutes = make(map[RouteKey]*RouteStruct)

// routeOperations add, update, delete
var routeOperations = Operations{Add: RouteAdded, Update: RouteUpdated, Delete: RouteDeleted}

// rtnType map of string key as RTN Type
var rtnType = map[string]int{
	"unspec":      unix.RTN_UNSPEC,
	"unicast":     unix.RTN_UNICAST,
	"local":       unix.RTN_LOCAL,
	"broadcast":   unix.RTN_BROADCAST,
	"anycast":     unix.RTN_ANYCAST,
	"multicast":   unix.RTN_MULTICAST,
	"blackhole":   unix.RTN_BLACKHOLE,
	"unreachable": unix.RTN_UNREACHABLE,
	"prohibit":    unix.RTN_PROHIBIT,
	"throw":       unix.RTN_THROW,
	"nat":         unix.RTN_NAT,
	"xresolve":    unix.RTN_XRESOLVE,
	"neighbor":    rtNNeighbor,
}

// rtnProto map of string key as RTN Type
var rtnProto = map[string]int{
	"unspec":        unix.RTPROT_UNSPEC,
	"redirect":      unix.RTPROT_REDIRECT,
	"kernel":        unix.RTPROT_KERNEL,
	"boot":          unix.RTPROT_BOOT,
	"static":        unix.RTPROT_STATIC,
	"bgp":           int('B'),
	"ipu_infra_mgr": int('I'),
	"196":           196,
}

// rtnScope map of string key as RTN scope
var rtnScope = map[string]int{
	"global":  unix.RT_SCOPE_UNIVERSE,
	"site":    unix.RT_SCOPE_SITE,
	"link":    unix.RT_SCOPE_LINK,
	"local":   unix.RT_SCOPE_HOST,
	"nowhere": unix.RT_SCOPE_NOWHERE,
}

var testFlag = map[int]string{
	unix.RTNH_F_ONLINK:    "onlink",
	unix.RTNH_F_PERVASIVE: "pervasive",
}

const (
	// Define each route type as a constant
	routeTypeConnected = "connected"
	routeTypeEvpnVxlan = "evpn-vxlan"
	routeTypeStatic    = "static"
	routeTypeBgp       = "bgp"
	routeTypeLocal     = "local"
	routeTypeNeighbor  = "neighbor"
)

// Event Operations
const (
	// RouteAdded event const
	RouteAdded = "route_added"
	// RouteUpdated event const
	RouteUpdated = "route_updated"
	// RouteDeleted event const
	RouteDeleted = "route_deleted"
)

// Route Direction
const ( // Route direction
	None int = iota
	RX
	TX
	RXTX
)

// RouteKey structure of route description
type RouteKey struct {
	Table int
	Dst   string
}

// RcNexthop structure of route nexthops
type RcNexthop struct {
	Gateway string
	Dev     string
	Flags   []string
	Weight  int
}

// RouteCmdInfo structure
type RouteCmdInfo struct {
	Type     string
	Dst      string
	Nhid     int
	Gateway  string
	Dev      string
	Protocol string
	Scope    string
	Prefsrc  string
	Metric   int
	Flags    []string
	Weight   int
	VRF      *infradb.Vrf
	Table    int
	NhInfo   NhRouteInfo // {id gateway Dev scope protocol flags}
	Nexthops []RcNexthop
}

/*--------------------------------------------------------------------------
###  Route Database Entries
###
###  In the internal Route table, there is one entry per VRF and IP prefix
###  to be installed in the routing table of the P4 pipeline. If there are
###  multiple routes in the Linux  route database for the same VRF and
###  prefix, we pick the one with the lowest metric (as does the Linux
###  forwarding plane).
###  The key of the internal Route table consists of (vrf, dst prefix) and
###  corresponds to the match fields in the P4 routing table. The rx/tx
###  direction match field of the MEV P4 pipeline and the necessary
###  duplication of some route entries is a technicality the MEV P4 pipeline
###  and must be handled by the p4ctrl module.
--------------------------------------------------------------------------*/

// RouteStruct structure has route info
type RouteStruct struct {
	Route0   vn.Route
	Vrf      *infradb.Vrf
	Nexthops []*NexthopStruct
	Metadata map[interface{}]interface{}
	NlType   string
	Key      RouteKey
	Err      error
}

// RouteList list has route info
type RouteList struct {
	RS []*RouteStruct
}

// VrfStatusGetter gets vrf status
type VrfStatusGetter interface {
	GetVrfOperStatus() infradb.VrfOperStatus
}

// readRoutes reads the routes
func readRoutes(v *infradb.Vrf) {
	readRouteFromIP(v)
}

// checkRoute checks the route
func (route *RouteStruct) checkRoute() bool {
	rk := route.Key
	for k := range latestRoutes {
		if k == rk {
			return true
		}
	}
	return false
}

// nolint
func (route *RouteStruct) setRouteType(v *infradb.Vrf) string {
	if route.Route0.Type == unix.RTN_UNICAST && route.Route0.Protocol == unix.RTPROT_KERNEL && route.Route0.Scope == unix.RT_SCOPE_LINK && len(route.Nexthops) == 1 {
		// Connected routes are proto=kernel and scope=link with a netdev as single nexthop
		return routeTypeConnected
	} else if route.Route0.Type == unix.RTN_UNICAST && int(route.Route0.Protocol) == int('B') && route.Route0.Scope == unix.RT_SCOPE_UNIVERSE {
		// EVPN routes to remote destinations are proto=bgp, scope global withipu_infra_mgr_db
		// all Nexthops residing on the br-<VRF name> bridge interface of the VRF.
		var devs []string
		if len(route.Nexthops) != 0 {
			for _, d := range route.Nexthops {
				devs = append(devs, nameIndex[d.nexthop.LinkIndex])
			}
			if len(devs) == 1 && devs[0] == "br-"+v.Name {
				return routeTypeEvpnVxlan
			}
			return routeTypeBgp
		}
	} else if route.Route0.Type == unix.RTN_UNICAST && checkProto(int(route.Route0.Protocol)) && route.Route0.Scope == unix.RT_SCOPE_UNIVERSE {
		return routeTypeStatic
	} else if route.Route0.Type == unix.RTN_LOCAL {
		return routeTypeLocal
	} else if route.Route0.Type == rtNNeighbor {
		// Special /32 or /128 routes for Resolved neighbors on connected subnets
		return routeTypeNeighbor
	}
	return "unknown"
}

// addRoute add the route
func (route *RouteStruct) addRoute() {
	ch := route.checkRoute()
	if ch {
		r0 := latestRoutes[route.Key]
		if route.Route0.Priority >= r0.Route0.Priority {
			// Route with lower metric exists and takes precedence
			log.Printf("netlink: Ignoring %+v  with higher metric than %+v\n", route, r0)
		} else {
			log.Printf("netlink: conflicts %+v with higher metric %+v. Will ignore it", route, r0)
		}
	} else {
		nexthops := route.Nexthops
		route.Nexthops = []*NexthopStruct{}
		for _, nexthop := range nexthops {
			nexthop.Metadata = make(map[interface{}]interface{})
			route = nexthop.addNexthop(route)
		}
		latestRoutes[route.Key] = route
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
		if len(r0.Nexthops) > 1 {
			for _, n := range r0.Nexthops {
				var nh NexthopStruct
				r0.Gateway = n.Gateway
				r0.Dev = n.Dev
				r0.Weight = n.Weight
				nh.ParseNexthop(v, r0)
				rs.Nexthops = append(rs.Nexthops, &nh)
			}
		} else if r0.Nhid != 0 || r0.Gateway != "" || r0.Dev != "" {
			var nh NexthopStruct
			nh.ParseNexthop(v, r0)
			rs.Nexthops = append(rs.Nexthops, &nh)
		}
		rs.NlType = "unknown"
		rs.Route0.Table = t
		rs.Route0.Priority = 1
		if r0.Dev != "" {
			dev, _ := vn.LinkByName(r0.Dev)
			rs.Route0.LinkIndex = dev.Attrs().Index
		}
		if r0.Dst != "" {
			var mask int
			split := r0.Dst
			if strings.Contains(r0.Dst, "/") {
				split4 := strings.Split(r0.Dst, "/")
				mask, _ = strconv.Atoi(split4[1])
				split = split4[0]
			} else {
				mask = 32
			}
			var nIP *net.IPNet
			if r0.Dst == "default" {
				nIP = &net.IPNet{
					IP:   net.ParseIP("0.0.0.0"),
					Mask: net.IPv4Mask(0, 0, 0, 0),
				}
			} else {
				mtoip := netMaskToInt(mask)
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
		rs.NlType = rs.setRouteType(v)
		rs.Key = RouteKey{Table: rs.Route0.Table, Dst: rs.Route0.Dst.String()}
		if rs.preFilterRoute() {
			route.RS = append(route.RS, &rs)
		} else if rs.checkRoute() {
			rou := latestRoutes[rs.Key]
			route.RS = append(route.RS, rou)
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
	for _, route := range v.Metadata.RoutingTable {
		rt = int(*route)
		raw, err := nlink.ReadRoute(ctx, strconv.Itoa(rt))
		if err != nil || len(raw) <= 3 {
			log.Printf("netlink: Err Command route\n")
			continue
		}
		var rawMessages []json.RawMessage
		err = json.Unmarshal([]byte(raw), &rawMessages)
		if err != nil {
			log.Printf("netlink route: JSON unmarshal error: %v %v : %v", err, raw, rawMessages)
			continue
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
				continue
			}
			routeData = append(routeData, ri)
		}
		rl = cmdProcessRt(v, routeData, rt)
		for _, r := range rl.RS {
			r.addRoute()
		}
	}
	nl := getNeighborRoutes() // Add extra routes for Resolved neighbors on connected subnets
	for i := 0; i < len(nl); i++ {
		rm = append(rm, nl[i])
	}
	nr := ParseRoute(v, rm, 0)
	for _, r := range nr.RS {
		r.addRoute()
	}
}

// getProto gets the route protocol
func (route *RouteStruct) getProto() string {
	for p, i := range rtnProto {
		if i == int(route.Route0.Protocol) {
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
		var routeTable map[RouteKey]*RouteStruct
		if len(latestRoutes) != 0 {
			routeTable = latestRoutes
		} else {
			routeTable = routes
		}
		rDB, ok := routeTable[r0.Key]
		if ok {
			// Return the existing route in the DB
			return rDB, ok
		}
		// Return the just constructed non-DB route
		return r0, true
	}

	log.Printf("netlink: Failed to lookup route %v in VRF %v", dst, v)
	return &RouteStruct{}, false
}

// checkRtype checks the route type
func checkRtype(rType string) bool {
	var types = map[string]struct{}{routeTypeConnected: {}, routeTypeEvpnVxlan: {}, routeTypeStatic: {}, routeTypeBgp: {}, routeTypeLocal: {}, routeTypeNeighbor: {}}
	if _, ok := types[rType]; ok {
		return true
	}
	return false
}

// installFilterRoute install the route filter
func (route *RouteStruct) installFilterRoute() bool {
	var nh []*NexthopStruct
	for _, n := range route.Nexthops {
		if n.Resolved {
			nh = append(nh, n)
		}
	}
	route.Nexthops = nh
	keep := checkRtype(route.NlType) && len(nh) != 0 && route.Route0.Dst.IP.String() != "0.0.0.0"
	return keep
}

// preFilterRoute pre filter the routes
func (route *RouteStruct) preFilterRoute() bool {
	if checkRtype(route.NlType) && !route.Route0.Dst.IP.IsLoopback() && route.Route0.Dst.IP.String() != "0.0.0.0" && !grdDefaultRoute {
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

// dumpRouteDB dump the route database
func dumpRouteDB() string {
	var s string

	s = "Route table:\n"
	for _, n := range routes {
		var via string
		if n.Route0.Gw == nil {
			via = strNone
		} else {
			via = n.Route0.Gw.String()
		}
		str := fmt.Sprintf("Route(vrf=%s dst=%s type=%s proto=%s metric=%d  via=%s dev=%s nhid= %+v Table= %d)", n.Vrf.Name, n.Route0.Dst.String(), n.NlType, n.getProto(), n.Route0.Priority, via, nameIndex[n.Route0.LinkIndex], n.Nexthops, n.Route0.Table)
		s += str
		s += "\n"
	}
	s += "\n\n"
	return s
}
