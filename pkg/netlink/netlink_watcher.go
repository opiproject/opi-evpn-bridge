// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package netlink handles the netlink related functionality
package netlink

import (
	"context"
	"fmt"
	"log"
	"os"

	"regexp"
	"strconv"
	"strings"
	"time"

	"encoding/binary"
	"encoding/json"
	"net"
	"reflect"
	"sync/atomic"

	"golang.org/x/sys/unix"

	vn "github.com/vishvananda/netlink"

	"path"

	"github.com/opiproject/opi-evpn-bridge/pkg/config"
	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	eb "github.com/opiproject/opi-evpn-bridge/pkg/netlink/eventbus"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
)

var ctx context.Context

var nlink utils.Netlink

// EventBus variable
var EventBus = eb.NewEventBus()

// pollInterval variable
var pollInterval int

// dbphyPortslock variable
var phyPorts = make(map[string]int)

// stopMonitoring variable
var stopMonitoring atomic.Value

// strNone variable
var strNone = "NONE"
var zebraStr = "zebra"

// l2nexthopID
var l2nexthopID = 16

// nhNextID Variable
var nhNextID = 16

// Route Direction
const ( // Route direction
	None int = iota
	RX
	TX
	RXTX
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

// rtNNeighbor
const (
	rtNNeighbor = 1111
)

// neighKey strcture of neighbor
type neighKey struct {
	Dst     string
	VrfName string
	Dev     int
}

// routeKey structure of route description
type routeKey struct {
	Table int
	Dst   string
}

// nexthopKey structure of nexthop
type nexthopKey struct {
	VrfName string
	Dst     string
	Dev     int
	Local   bool
}

// fdbKey structure key for sorting theFDB entries
type fdbKey struct {
	VlanID int
	Mac    string
}

// l2NexthopKey is l2 neighbor key
type l2NexthopKey struct {
	Dev    string
	VlanID int
	Dst    string
}

// neighIPStruct nighbor ip structure
type neighIPStruct struct {
	Dst         string
	Dev         string
	Lladdr      string
	ExternLearn string
	State       []string
	Protocol    string
}

// fdbIPStruct fdb ip structure
type fdbIPStruct struct {
	Mac    string
	Ifname string
	Vlan   int
	Flags  []string
	Master string
	State  string
	Dst    string
}

// nHRouteInfo neighbor route info
type nHRouteInfo struct {
	ID       int
	Gateway  string
	Dev      string
	Scope    string
	Protocol string
	Flags    []string
}

// routeCmdInfo structure
type routeCmdInfo struct {
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
	NhInfo   nHRouteInfo // {id gateway Dev scope protocol flags}
}

// routes Variable
var routes = make(map[routeKey]*RouteStruct)

// Nexthops Variable
var Nexthops = make(map[nexthopKey]*NexthopStruct)

// Neighbors Variable
var Neighbors = make(map[neighKey]neighStruct)

// fDB Variable
var fDB = make(map[fdbKey]*FdbEntryStruct)

// l2Nexthops Variable
var l2Nexthops = make(map[l2NexthopKey]*L2NexthopStruct)

// latestRoutes Variable
var latestRoutes = make(map[routeKey]*RouteStruct)

// latestNexthop Variable
var latestNexthop = make(map[nexthopKey]*NexthopStruct)

// latestNeighbors Variable
var latestNeighbors = make(map[neighKey]neighStruct)

// latestFDB Variable
var latestFDB = make(map[fdbKey]*FdbEntryStruct)

// latestL2Nexthop Variable
var latestL2Nexthop = make(map[l2NexthopKey]*L2NexthopStruct)

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
	Key      routeKey
	Err      error
}

// routeList list has route info
type routeList struct {
	RS []*RouteStruct
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
	Key       nexthopKey
	Resolved  bool
	Neighbor  *neighStruct
	NhType    int
	Metadata  map[interface{}]interface{}
}

// --------------------------------------------------------------------------
// ###  Bridge MAC Address Database
// ###
// ###  We split the Linux fDB entries into DMAC and L2 NexthopStruct tables similar
// ###  to routes and L3 nexthops, Thus, all remote EVPN DMAC entries share a
// ###  single VXLAN L2 nexthop table entry.
// ###
// ###  TODO: Support for dynamically learned MAC addresses on BridgePorts
// ###  (e.g. for pod interfaces operating in promiscuous mode).
// --------------------------------------------------------------------------

// L2NexthopStruct structure
type L2NexthopStruct struct {
	Dev      string
	VlanID   int
	Dst      net.IP
	Key      l2NexthopKey
	lb       *infradb.LogicalBridge
	bp       *infradb.BridgePort
	ID       int
	FdbRefs  []*FdbEntryStruct
	Resolved bool
	Type     int
	Metadata map[interface{}]interface{}
}

// FdbEntryStruct structure
type FdbEntryStruct struct {
	VlanID   int
	Mac      string
	Key      fdbKey
	State    string
	lb       *infradb.LogicalBridge
	bp       *infradb.BridgePort
	Nexthop  *L2NexthopStruct
	Type     int
	Metadata map[interface{}]interface{}
	Err      error
}

// neighStruct structure
type neighStruct struct {
	Neigh0   vn.Neigh
	Protocol string
	VrfName  string
	Type     int
	Dev      string
	Err      error
	Key      neighKey
	Metadata map[interface{}]interface{}
}

// neighList structure
type neighList struct {
	NS []neighStruct
}

// netMaskToInt converts a CIDR network mask (e.g., 24 for a /24 subnet) to a 4-octet netmask.
func netMaskToInt(mask int) (netmaskint [4]uint8) {
	// Perform initial validation and parse the CIDR using a dummy IP.
	_, ipv4Net, err := net.ParseCIDR(fmt.Sprintf("0.0.0.0/%d", mask))
	if err != nil {
		return [4]uint8{}
	}

	// Initialize an array to hold the subnet mask.
	var maskArray [4]uint8
	copy(maskArray[:], ipv4Net.Mask)

	return maskArray
}

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

//--------------------------------------------------------------------------
//###  Neighbor Database Entries
//--------------------------------------------------------------------------

// linkTable wg sync.WaitGroup
var linkTable []vn.Link

// vrfList netlink libarary var
var vrfList []vn.Link

// deviceList netlink libarary var
var deviceList []vn.Link

// vlanList netlink libarary var
var vlanList []vn.Link

// bridgeList netlink libarary var
var bridgeList []vn.Link

// vxlanList netlink libarary var
var vxlanList []vn.Link

// linkList netlink libarary var
var linkList []vn.Link

// NameIndex netlink libarary var
var NameIndex = make(map[int]string)

// oldgenmap old map
var oldgenmap = make(map[interface{}]interface{})

// latestgenmap latest map
var latestgenmap = make(map[interface{}]interface{})

// notifyEvents array
var notifyEvents = []string{"_added", "_updated", "_deleted"}

//--------------------------------------------------------------------------
//###  NexthopStruct Database Entries
//--------------------------------------------------------------------------

// l2NhIDCache
var l2NhIDCache = make(map[l2NexthopKey]int)

// nhIDCache Variable
var nhIDCache = make(map[nexthopKey]int)

const (
	// Define each route type as a constant
	routeTypeConnected = "connected"
	routeTypeEvpnVxlan = "evpn-vxlan"
	routeTypeStatic    = "static"
	routeTypeBgp       = "bgp"
	routeTypeLocal     = "local"
	routeTypeNeighbor  = "neighbor"
)

const (
	// RouteAdded event const
	RouteAdded = "route_added"
	// RouteUpdated event const
	RouteUpdated = "route_updated"
	// RouteDeleted event const
	RouteDeleted = "route_deleted"
	// NexthopAdded event const
	NexthopAdded = "nexthop_added"
	// NexthopUpdated event const
	NexthopUpdated = "nexthop_updated"
	// NexthopDeleted event const
	NexthopDeleted = "nexthop_deleted"
	// FdbEntryAdded event const
	FdbEntryAdded = "fdb_entry_added"
	// FdbEntryUpdated event const
	FdbEntryUpdated = "fdb_entry_updated"
	// FdbEntryDeleted event const
	FdbEntryDeleted = "fdb_entry_deleted"
	// L2NexthopAdded event const
	L2NexthopAdded = "l2_nexthop_added"
	// L2NexthopUpdated event const
	L2NexthopUpdated = "l2_nexthop_updated"
	// L2NexthopDeleted event const
	L2NexthopDeleted = "l2_nexthop_deleted"
)

// getFlag gets the flag
func getFlag(s string) int {
	f := 0
	for ff, ss := range testFlag {
		if s == ss {
			f |= ff
		}
	}
	return f
}

// getFlagString return flag of type string
func getFlagString(flag int) string {
	str, ok := testFlag[flag]
	if !ok {
		return ""
	}
	return str
}

// NHAssignID returns the nexthop id
func NHAssignID(key nexthopKey) int {
	id := nhIDCache[key]
	if id == 0 {
		// Assigne a free id and insert it into the cache
		id = nhNextID
		nhIDCache[key] = id
		nhNextID++
	}
	return id
}

// NHParse parses the neighbor
func NHParse(v *infradb.Vrf, rc routeCmdInfo) *NexthopStruct {
	var nh NexthopStruct
	nh.Weight = 1
	nh.Vrf = v
	if rc.Dev != "" {
		vrf, _ := vn.LinkByName(rc.Dev)
		nh.nexthop.LinkIndex = vrf.Attrs().Index
		NameIndex[nh.nexthop.LinkIndex] = vrf.Attrs().Name
	}
	if len(rc.Flags) != 0 {
		nh.nexthop.Flags = getFlag(rc.Flags[0])
	}
	if rc.Gateway != "" {
		nIP := &net.IPNet{
			IP: net.ParseIP(rc.Gateway),
		}
		nh.nexthop.Gw = nIP.IP
	}
	if rc.Protocol != "" {
		nh.Protocol = rtnProto[rc.Protocol]
	}
	if rc.Scope != "" {
		nh.Scope = rtnScope[rc.Scope]
	}
	if rc.Type != "" {
		nh.NhType = rtnType[rc.Type]
		if nh.NhType == unix.RTN_LOCAL {
			nh.Local = true
		} else {
			nh.Local = false
		}
	}
	if rc.Weight >= 0 {
		nh.Weight = rc.Weight
	}
	nh.Key = nexthopKey{nh.Vrf.Name, nh.nexthop.Gw.String(), nh.nexthop.LinkIndex, nh.Local}
	return &nh
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

// preFilterRoute pre filter the routes
func preFilterRoute(r *RouteStruct) bool {
	if checkRtype(r.NlType) && !r.Route0.Dst.IP.IsLoopback() && r.Route0.Dst.IP.String() != "0.0.0.0" {
		return true
	}

	return false
}

// checkProto checks the proto type
func checkProto(proto int) bool {
	var protos = [3]int{unix.RTPROT_BOOT, unix.RTPROT_STATIC, 196}
	for _, v := range protos {
		if proto == v {
			return true
		}
	}
	return false
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
				devs = append(devs, NameIndex[d.nexthop.LinkIndex])
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

// ParseRoute parse the routes
// nolint
func ParseRoute(v *infradb.Vrf, Rm []routeCmdInfo, t int) routeList {
	var route routeList
	for _, Ro := range Rm {
		if Ro.Type == "" && (Ro.Dev != "" || Ro.Gateway != "") {
			Ro.Type = routeTypeLocal
		}
		var rs RouteStruct
		rs.Vrf = v
		if Ro.Nhid != 0 || Ro.Gateway != "" || Ro.Dev != "" {
			rs.Nexthops = append(rs.Nexthops, NHParse(v, Ro))
		}
		rs.NlType = "unknown"
		rs.Route0.Table = t
		rs.Route0.Priority = 1
		if Ro.Dev != "" {
			dev, _ := vn.LinkByName(Ro.Dev)
			rs.Route0.LinkIndex = dev.Attrs().Index
		}
		if Ro.Dst != "" {
			var Mask int
			split := Ro.Dst
			if strings.Contains(Ro.Dst, "/") {
				split4 := strings.Split(Ro.Dst, "/")
				Mask, _ = strconv.Atoi(split4[1])
				split = split4[0]
			} else {
				Mask = 32
			}
			var nIP *net.IPNet
			if Ro.Dst == "default" {
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
		if Ro.Metric != 0 {
			rs.Route0.Priority = Ro.Metric
		}
		if Ro.Protocol != "" {
			if rtnProto[Ro.Protocol] != 0 {
				rs.Route0.Protocol = vn.RouteProtocol(rtnProto[Ro.Protocol])
			} else {
				rs.Route0.Protocol = 0
			}
		}
		if Ro.Type != "" {
			rs.Route0.Type = rtnType[Ro.Type]
		}
		if len(Ro.Flags) != 0 {
			rs.Route0.Flags = getFlag(Ro.Flags[0])
		}
		if Ro.Scope != "" {
			rs.Route0.Scope = vn.Scope(rtnScope[Ro.Scope])
		}
		if Ro.Prefsrc != "" {
			nIP := &net.IPNet{
				IP: net.ParseIP(Ro.Prefsrc),
			}
			rs.Route0.Src = nIP.IP
		}
		if Ro.Gateway != "" {
			nIP := &net.IPNet{
				IP: net.ParseIP(Ro.Gateway),
			}
			rs.Route0.Gw = nIP.IP
		}
		if Ro.VRF != nil {
			rs.Vrf, _ = infradb.GetVrf(Ro.VRF.Name)
		}
		if Ro.Table != 0 {
			rs.Route0.Table = Ro.Table
		}
		rs.NlType = setRouteType(&rs, v)
		rs.Key = routeKey{Table: rs.Route0.Table, Dst: rs.Route0.Dst.String()}
		if preFilterRoute(&rs) {
			route.RS = append(route.RS, &rs)
		}
	}
	return route
}

// ParseFdb parse the fdb
func ParseFdb(fdbIP fdbIPStruct) *FdbEntryStruct {
	var fdbentry FdbEntryStruct
	fdbentry.VlanID = fdbIP.Vlan
	fdbentry.Mac = fdbIP.Mac
	fdbentry.Key = fdbKey{fdbIP.Vlan, fdbIP.Mac}
	fdbentry.State = fdbIP.State
	fdbentry.Nexthop = &L2NexthopStruct{}
	lbs, _ := infradb.GetAllLBs()
	for _, lb := range lbs {
		if lb.Spec.VlanID == uint32(fdbentry.VlanID) {
			fdbentry.lb = lb
			break
		}
	}
	if fdbentry.lb != nil {
		bp := fdbentry.lb.MacTable[fdbentry.Mac]
		if bp != "" {
			fdbentry.bp, _ = infradb.GetBP(bp)
		}
	}
	Dev := fdbIP.Ifname
	dst := fdbIP.Dst
	fdbentry.Nexthop.ParseL2NH(fdbentry.VlanID, Dev, dst, fdbentry.lb, fdbentry.bp)
	fdbentry.Type = fdbentry.Nexthop.Type
	return &fdbentry
}

// nolint
// ParseL2NH parse the l2hn
func (l2n *L2NexthopStruct) ParseL2NH(vlanID int, dev string, dst string, LB *infradb.LogicalBridge, BP *infradb.BridgePort) {
	l2n.Dev = dev
	l2n.VlanID = vlanID
	l2n.Dst = net.IP(dst)
	l2n.Key = l2NexthopKey{l2n.Dev, l2n.VlanID, string(l2n.Dst)}
	l2n.lb = LB
	l2n.bp = BP
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
func L2NHAssignID(key l2NexthopKey) int {
	id := l2NhIDCache[key]
	if id == 0 {
		// Assigne a free id and insert it into the cache
		id = l2nexthopID
		l2NhIDCache[key] = id
		l2nexthopID++
	}
	return id
}

// addFdbEntry add fdb entries
func addFdbEntry(m *FdbEntryStruct) {
	m = addL2Nexthop(m)
	// TODO
	// logger.debug(f"Adding {m.format()}.")
	latestFDB[m.Key] = m
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

// getlink get the link
func getlink() {
	links, err := vn.LinkList()
	if err != nil {
		log.Fatal("netlink:", err)
	}
	for i := 0; i < len(links); i++ {
		linkTable = append(linkTable, links[i])
		NameIndex[links[i].Attrs().Index] = links[i].Attrs().Name
		switch links[i].Type() {
		case "vrf":
			vrfList = append(vrfList, links[i])
		case "device":
			deviceList = append(deviceList, links[i])
		case "vlan":
			vlanList = append(vlanList, links[i])
		case "bridge":
			bridgeList = append(bridgeList, links[i])
		case "vxlan":
			vxlanList = append(vxlanList, links[i])
		default:
		}
		linkList = append(linkList, links[i])
	}
}

// readLatestNetlinkState reads the latest netlink state
func readLatestNetlinkState() {
	vrfs, _ := infradb.GetAllVrfs()
	for _, v := range vrfs {
		readNeighbors(v) // viswanantha library
		readRoutes(v)    // Viswantha library
	}
	m := readFDB()
	log.Printf("netlink: len(m) :%v\n", len(m))
	for i := 0; i < len(m); i++ {
		addFdbEntry(m[i])
	}
	dumpDBs()
}

// dumpDBs dumps the databse
func dumpDBs() {
	file, err := os.OpenFile("netlink_dump", os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}
	if err := os.Truncate("netlink_dump", 0); err != nil {
		log.Printf("netlink: Failed to truncate: %v", err)
	}
	str := dumpRouteDB()
	log.Printf("\n")
	str += dumpNexthDB()
	log.Printf("\n")
	str += dumpNeighDB()
	log.Printf("\n")
	str += dumpFDB()
	log.Printf("\n")
	str += dumpL2NexthDB()
	_, err = file.WriteString(str)
	if err != nil {
		log.Printf("netlink: %v", err)
	}
	err = file.Close()
	if err != nil {
		log.Printf("netlink: error closing file: %v", err)
	}
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

// CheckNdup checks the duplication of neighbor
func CheckNdup(tmpKey neighKey) bool {
	var dup = false
	for k := range latestNeighbors {
		if k == tmpKey {
			dup = true
			break
		}
	}
	return dup
}

// CheckRdup checks the duplication of routes
func CheckRdup(tmpKey routeKey) bool {
	var dup = false
	for j := range latestRoutes {
		if j == tmpKey {
			dup = true
			break
		}
	}
	return dup
}

// addNeigh adds the neigh
func addNeigh(dump neighList) {
	for _, n := range dump.NS {
		n = neighborAnnotate(n)
		if len(latestNeighbors) == 0 {
			latestNeighbors[n.Key] = n
		} else if !CheckNdup(n.Key) {
			latestNeighbors[n.Key] = n
		}
	}
}

// getStateStr gets the state from int
func getStateStr(s int) string {
	neighState := map[int]string{
		vn.NUD_NONE:       "NONE",
		vn.NUD_INCOMPLETE: "INCOMPLETE",
		vn.NUD_REACHABLE:  "REACHABLE",
		vn.NUD_STALE:      "STALE",
		vn.NUD_DELAY:      "DELAY",
		vn.NUD_PROBE:      "PROBE",
		vn.NUD_FAILED:     "FAILED",
		vn.NUD_NOARP:      "NOARP",
		vn.NUD_PERMANENT:  "PERMANENT",
	}
	return neighState[s]
}

func printNeigh(neigh *neighStruct) string {
	var Proto string
	if neigh == nil {
		return strNone
	}
	if neigh.Protocol == "" {
		Proto = strNone
	} else {
		Proto = neigh.Protocol
	}
	str := fmt.Sprintf("Neighbor(vrf=%s dst=%s lladdr=%s dev=%s proto=%s state=%s) ", neigh.VrfName, neigh.Neigh0.IP.String(), neigh.Neigh0.HardwareAddr.String(), NameIndex[neigh.Neigh0.LinkIndex], Proto, getStateStr(neigh.Neigh0.State))
	return str
}

// dumpRouteDB dump the route database
func dumpRouteDB() string {
	var s string
	log.Printf("netlink: Dump Route table:\n")
	s = "Route table:\n"
	for _, n := range latestRoutes {
		var via string
		if n.Route0.Gw.String() == "<nil>" {
			via = strNone
		} else {
			via = n.Route0.Gw.String()
		}
		str := fmt.Sprintf("Route(vrf=%s dst=%s type=%s proto=%s metric=%d  via=%s dev=%s nhid= %d Table= %d)", n.Vrf.Name, n.Route0.Dst.String(), n.NlType, getProto(n), n.Route0.Priority, via, NameIndex[n.Route0.LinkIndex], n.Nexthops[0].ID, n.Route0.Table)
		log.Println(str)
		s += str
		s += "\n"
	}
	log.Printf("\n\n\n")
	s += "\n\n"
	return s
}

// dumpL2NexthDB dump the l2 nexthop entries
func dumpL2NexthDB() string {
	var s string
	log.Printf("netlink: Dump L2 Nexthop table:\n")
	s = "L2 Nexthop table:\n"
	var ip string
	for _, n := range latestL2Nexthop {
		if n.Dst.String() == "<nil>" {
			ip = strNone
		} else {
			ip = n.Dst.String()
		}
		str := fmt.Sprintf("L2Nexthop(id=%d dev=%s vlan=%d dst=%s type=%d #fDB entries=%d Resolved=%t) ", n.ID, n.Dev, n.VlanID, ip, n.Type, len(n.FdbRefs), n.Resolved)
		log.Println(str)
		s += str
		s += "\n"
	}
	log.Printf("\n\n\n")
	s += "\n\n"
	return s
}

// dumpFDB dump the fdb entries
func dumpFDB() string {
	var s string
	log.Printf("netlink: Dump fDB table:\n")
	s = "fDB table:\n"
	for _, n := range latestFDB {
		str := fmt.Sprintf("MacAddr(vlan=%d mac=%s state=%s type=%d l2nh_id=%d) ", n.VlanID, n.Mac, n.State, n.Type, n.Nexthop.ID)
		log.Println(str)
		s += str
		s += "\n"
	}
	log.Printf("\n\n\n")
	s += "\n\n"
	return s
}

// dumpNexthDB dump the nexthop entries
func dumpNexthDB() string {
	var s string
	log.Printf("netlink: Dump Nexthop table:\n")
	s = "Nexthop table:\n"
	for _, n := range latestNexthop {
		str := fmt.Sprintf("Nexthop(id=%d vrf=%s dst=%s dev=%s Local=%t weight=%d flags=[%s] #routes=%d Resolved=%t neighbor=%s) ", n.ID, n.Vrf.Name, n.nexthop.Gw.String(), NameIndex[n.nexthop.LinkIndex], n.Local, n.Weight, getFlagString(n.nexthop.Flags), len(n.RouteRefs), n.Resolved, printNeigh(n.Neighbor))
		log.Println(str)
		s += str
		s += "\n"
	}
	log.Printf("\n\n\n")
	s += "\n\n"
	return s
}

// dumpNeighDB dump the neighbor entries
func dumpNeighDB() string {
	var s string
	log.Printf("netlink: Dump Neighbor table:\n")
	s = "Neighbor table:\n"
	for _, n := range latestNeighbors {
		var Proto string
		if n.Protocol == "" {
			Proto = strNone
		} else {
			Proto = n.Protocol
		}
		str := fmt.Sprintf("Neighbor(vrf=%s dst=%s lladdr=%s dev=%s proto=%s state=%s Type : %d) ", n.VrfName, n.Neigh0.IP.String(), n.Neigh0.HardwareAddr.String(), NameIndex[n.Neigh0.LinkIndex], Proto, getStateStr(n.Neigh0.State), n.Type)
		log.Println(str)
		s += str
		s += "\n"
	}
	s += "\n\n"
	return s
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

// checkNeigh checks the nighbor
func checkNeigh(nk neighKey) bool {
	for k := range latestNeighbors {
		if k == nk {
			return true
		}
	}
	return false
}

// tryResolve resolves the neighbor
func tryResolve(nexhthopSt *NexthopStruct) *NexthopStruct {
	if !reflect.ValueOf(nexhthopSt.nexthop.Gw).IsZero() {
		// Nexthops with a gateway IP need resolution of that IP
		neighborKey := neighKey{Dst: nexhthopSt.nexthop.Gw.String(), VrfName: nexhthopSt.Vrf.Name, Dev: nexhthopSt.nexthop.LinkIndex}
		ch := checkNeigh(neighborKey)
		if ch && latestNeighbors[neighborKey].Type != IGNORE {
			nexhthopSt.Resolved = true
			nh := latestNeighbors[neighborKey]
			nexhthopSt.Neighbor = &nh
		} else {
			nexhthopSt.Resolved = false
		}
	} else {
		nexhthopSt.Resolved = true
	}
	return nexhthopSt
}

// checkNhDB checks the neighbor database
func checkNhDB(nhKey nexthopKey) bool {
	for k := range latestNexthop {
		if k == nhKey {
			return true
		}
	}
	return false
}

// addNexthop adds the nexthop
func addNexthop(nexthop *NexthopStruct, r *RouteStruct) *RouteStruct {
	ch := checkNhDB(nexthop.Key)
	if ch {
		NH0 := latestNexthop[nexthop.Key]
		// Links route with existing nexthop
		NH0.RouteRefs = append(NH0.RouteRefs, r)
		r.Nexthops = append(r.Nexthops, NH0)
	} else {
		// Create a new nexthop entry
		nexthop.RouteRefs = append(nexthop.RouteRefs, r)
		nexthop.ID = NHAssignID(nexthop.Key)
		nexthop = tryResolve(nexthop)
		latestNexthop[nexthop.Key] = nexthop
		r.Nexthops = append(r.Nexthops, nexthop)
	}
	return r
}

// checkRoute checks the route
func checkRoute(r *RouteStruct) bool {
	Rk := r.Key
	for k := range latestRoutes {
		if k == Rk {
			return true
		}
	}
	return false
}

// deleteNH deletes the neighbor
func deleteNH(nexthop []*NexthopStruct) []*NexthopStruct {
	index := len(nexthop)
	if index == 1 {
		nexthop = append(nexthop[:0], nexthop[1:]...)
	} else {
		for i := 0; i < index-1; i++ {
			nexthop = append(nexthop[:0], nexthop[1:]...)
		}
	}
	return nexthop
}

// addRoute add the route
func addRoute(r *RouteStruct) {
	ch := checkRoute(r)
	if ch {
		R0 := latestRoutes[r.Key]
		if r.Route0.Priority >= R0.Route0.Priority {
			// Route with lower metric exists and takes precedence
			log.Printf("netlink: Ignoring %+v  with higher metric than %+v\n", r, R0)
		} else {
			log.Printf("netlink: conflicts %+v with higher metric %+v. Will ignore it", r, R0)
		}
	} else {
		Nexthops := r.Nexthops
		r.Nexthops = deleteNH(r.Nexthops)
		for _, nexthop := range Nexthops {
			r = addNexthop(nexthop, r)
		}
		latestRoutes[r.Key] = r
	}
}

// cmdProcessNb process the neighbor command
func cmdProcessNb(nb string, v string) neighList {
	var nbs []neighIPStruct
	CPs := strings.Split(nb[2:len(nb)-3], "},{")
	for i := 0; i < len(CPs); i++ {
		var ni neighIPStruct
		log.Println(CPs[i])
		err := json.Unmarshal([]byte(fmt.Sprintf("{%v}", CPs[i])), &ni)
		if err != nil {
			log.Println("netlink: error-", err)
		}
		nbs = append(nbs, ni)
	}
	Neigh := parseNeigh(nbs, v)
	return Neigh
}

// getState gets the state for the neighbor
func getState(s string) int {
	neighState := map[string]int{
		"NONE":       vn.NUD_NONE,
		"INCOMPLETE": vn.NUD_INCOMPLETE,
		"REACHABLE":  vn.NUD_REACHABLE,
		"STALE":      vn.NUD_STALE,
		"DELAY":      vn.NUD_DELAY,
		"PROBE":      vn.NUD_PROBE,
		"FAILED":     vn.NUD_FAILED,
		"NOARP":      vn.NUD_NOARP,
		"PERMANENT":  vn.NUD_PERMANENT,
	}
	return neighState[s]
}

// preFilterNeighbor pre filter the neighbors
func preFilterNeighbor(n neighStruct) bool {
	if n.Neigh0.State != vn.NUD_NONE && n.Neigh0.State != vn.NUD_INCOMPLETE && n.Neigh0.State != vn.NUD_FAILED && NameIndex[n.Neigh0.LinkIndex] != "lo" {
		return true
	}

	return false
}

// parseNeigh parses the neigh
func parseNeigh(nm []neighIPStruct, v string) neighList {
	var NL neighList
	for _, ND := range nm {
		var ns neighStruct
		ns.Neigh0.Type = OTHER
		ns.VrfName = v
		if ND.Dev != "" {
			vrf, _ := vn.LinkByName(ND.Dev)
			ns.Neigh0.LinkIndex = vrf.Attrs().Index
		}
		if ND.Dst != "" {
			ipnet := &net.IPNet{
				IP: net.ParseIP(ND.Dst),
			}
			ns.Neigh0.IP = ipnet.IP
		}
		if len(ND.State) != 0 {
			ns.Neigh0.State = getState(ND.State[0])
		}
		if ND.Lladdr != "" {
			ns.Neigh0.HardwareAddr, _ = net.ParseMAC(ND.Lladdr)
		}
		if ND.Protocol != "" {
			ns.Protocol = ND.Protocol
		}
		//	ns  =  neighborAnnotate(ns)   /* Need InfraDB to finish for fetching LB/BP information */
		ns.Key = neighKey{VrfName: v, Dst: ns.Neigh0.IP.String(), Dev: ns.Neigh0.LinkIndex}
		if preFilterNeighbor(ns) {
			NL.NS = append(NL.NS, ns)
		}
	}
	return NL
}

// getNeighborRoutes gets the nighbor routes
func getNeighborRoutes() []routeCmdInfo { // []map[string]string{
	// Return a list of /32 or /128 routes & Nexthops to be inserted into
	// the routing tables for Resolved neighbors on connected subnets
	// on physical and SVI interfaces.
	var neighborRoutes []routeCmdInfo // []map[string]string
	for _, N := range latestNeighbors {
		if N.Type == PHY || N.Type == SVI || N.Type == VXLAN {
			vrf, _ := infradb.GetVrf(N.VrfName)
			table := int(*vrf.Metadata.RoutingTable[0])

			//# Create a special route with dst == gateway to resolve
			//# the nexthop to the existing neighbor
			R0 := routeCmdInfo{Type: routeTypeNeighbor, Dst: N.Neigh0.IP.String(), Protocol: "ipu_infra_mgr", Scope: "global", Gateway: N.Neigh0.IP.String(), Dev: NameIndex[N.Neigh0.LinkIndex], VRF: vrf, Table: table}
			neighborRoutes = append(neighborRoutes, R0)
		}
	}
	return neighborRoutes
}

// readNeighbors reads the nighbors
func readNeighbors(v *infradb.Vrf) {
	var N neighList
	var err error
	var Nb string
	if v.Spec.Vni == nil {
		/* No support for "ip neighbor show" command in netlink library Raised ticket https://github.com/vishvananda/netlink/issues/913 ,
		   so using ip command as WA */
		Nb, err = nlink.ReadNeigh(ctx, "")
	} else {
		Nb, err = nlink.ReadNeigh(ctx, path.Base(v.Name))
	}
	if len(Nb) != 3 && err == nil {
		N = cmdProcessNb(Nb, v.Name)
	}
	addNeigh(N)
}

// preFilterMac filter the mac
func preFilterMac(f *FdbEntryStruct) bool {
	// TODO m.nexthop.dst
	if f.VlanID != 0 || (f.Nexthop.Dst != nil && !f.Nexthop.Dst.IsUnspecified()) {
		log.Printf("netlink: %d vlan \n", len(f.Nexthop.Dst.String()))
		return true
	}
	return false
}

// cmdProcessRt process the route command
func cmdProcessRt(v *infradb.Vrf, r string, t int) routeList {
	var RouteData []routeCmdInfo
	if len(r) <= 3 {
		log.Println("netlink: Error in the cmd:", r)
		var route routeList
		return route
	}
	CPs := strings.Split(r[2:len(r)-3], "},{")
	for i := 0; i < len(CPs); i++ {
		var ri routeCmdInfo
		log.Println(CPs[i])
		err := json.Unmarshal([]byte(fmt.Sprintf("{%v}", CPs[i])), &ri)
		if err != nil {
			log.Println("error-", err)
		}
		RouteData = append(RouteData, ri)
	}
	route := ParseRoute(v, RouteData, t)
	return route
}

// readRouteFromIP reads the routes from ip
func readRouteFromIP(v *infradb.Vrf) {
	var Rl routeList
	var rm []routeCmdInfo
	var Rt1 int
	for _, routeSt := range v.Metadata.RoutingTable {
		Rt1 = int(*routeSt)
		Raw, err := nlink.ReadRoute(ctx, strconv.Itoa(Rt1))
		if err != nil {
			log.Printf("netlink: Err Command route\n")
			return
		}
		Rl = cmdProcessRt(v, Raw, Rt1)
		for _, r := range Rl.RS {
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

// readRoutes reads the routes
func readRoutes(v *infradb.Vrf) {
	readRouteFromIP(v)
}

func notifyAddDel(r interface{}, event string) {
	log.Printf("netlink: Notify event: %s\n", event)
	EventBus.Publish(event, r)
}

func deepEqualN(n1 *NexthopStruct, n2 *NexthopStruct, nc bool) bool {
	if !reflect.DeepEqual(n1.Vrf.Name, n2.Vrf.Name) {
		return false
	}
	if !reflect.DeepEqual(n1.Weight, n2.Weight) {
		return false
	}
	if !reflect.DeepEqual(n1.ID, n2.ID) {
		return false
	}
	if !reflect.DeepEqual(n1.nexthop, n2.nexthop) {
		return false
	}
	if !reflect.DeepEqual(n1.Key, n2.Key) {
		return false
	}
	if !reflect.DeepEqual(n1.Local, n2.Local) {
		return false
	}
	if !reflect.DeepEqual(n1.Metadata, n2.Metadata) {
		return false
	}
	if !reflect.DeepEqual(n1.Metric, n2.Metric) {
		return false
	}
	if !reflect.DeepEqual(n1.Scope, n2.Scope) {
		return false
	}
	if !reflect.DeepEqual(n1.Resolved, n2.Resolved) {
		return false
	}
	if !reflect.DeepEqual(n1.Protocol, n2.Protocol) {
		return false
	}
	if !reflect.DeepEqual(n1.NhType, n2.NhType) {
		return false
	}
	if nc {
		if len(n1.RouteRefs) != len(n2.RouteRefs) {
			return false
		}
		for i := range n1.RouteRefs {
			ret := deepEqualR(n1.RouteRefs[i], n2.RouteRefs[i], false)
			if !ret {
				return false
			}
		}
	}
	/*if !reflect.DeepEqual(N1.Neighbor, N2.Neighbor) {
		return false
	}*/
	return true
}

func deepEqualR(r1 *RouteStruct, r2 *RouteStruct, nc bool) bool {
	if !reflect.DeepEqual(r1.Vrf.Name, r2.Vrf.Name) {
		return false
	}
	if !reflect.DeepEqual(r1.Route0, r2.Route0) {
		return false
	}
	if !reflect.DeepEqual(r1.Key, r2.Key) {
		return false
	}
	if !reflect.DeepEqual(r1.NlType, r2.NlType) {
		return false
	}
	if !reflect.DeepEqual(r1.Metadata, r2.Metadata) {
		return false
	}
	if nc {
		if len(r1.Nexthops) != len(r2.Nexthops) {
			return false
		}
		return deepEqualN(r1.Nexthops[0], r2.Nexthops[0], false)
	}
	return true
}

func deepCheck(v1 interface{}, v2 interface{}, event []string) bool {
	if strings.HasPrefix(event[1], "route") {
		r1 := v1.(*RouteStruct)
		r2 := v2.(*RouteStruct)
		return deepEqualR(r1, r2, true)
	} else if strings.HasPrefix(event[1], "nexthop") {
		n1 := v1.(*NexthopStruct)
		n2 := v2.(*NexthopStruct)
		return deepEqualN(n1, n2, true)
	}
	return true
}

// nolint
func notify_changes(new_db map[interface{}]interface{}, old_db map[interface{}]interface{}, event []string) {
	DB2 := old_db
	DB1 := new_db
	/* Checking the Updated entries in the netlink db by comparing the individual keys and their corresponding values in old and new db copies
	   entries with same keys with different values and send the notification to vendor specific module */
	for k1, v1 := range DB1 {
		for k2, v2 := range DB2 {
			if k1 == k2 {
				if !deepCheck(v1, v2, event) {
					// To Avoid in-correct update notification due to race condition in which metadata is nil in new entry and crashing in dcgw module
					if strings.Contains(event[1], "route") || strings.HasPrefix(event[1], "nexthop") {
						var rv *RouteStruct
						var nv *NexthopStruct
						if strings.Contains(event[1], "route") {
							rv = v1.(*RouteStruct)
							if rv.Vrf.Status.VrfOperStatus == infradb.VrfOperStatusToBeDeleted {
								notifyAddDel(rv, event[2])
								delete(new_db, k1)
								delete(old_db, k2)
								break
							}
						} else if strings.Contains(event[1], "nexthop") {
							nv = v1.(*NexthopStruct)
							if nv.Vrf.Status.VrfOperStatus == infradb.VrfOperStatusToBeDeleted {
								notifyAddDel(nv, event[2])
								delete(new_db, k1)
								delete(old_db, k2)
								break
							}
						}
					}
					notifyAddDel(v1, event[1])
				}
				delete(new_db, k1)
				delete(old_db, k2)
				break
			}
		}
	}
	for _, r := range new_db { // Added entries notification cases
		notifyAddDel(r, event[0])
	}
	for _, r := range old_db { // Deleted entires notification cases
		notifyAddDel(r, event[2])
	}
}

// readFDB read the fdb from db
func readFDB() []*FdbEntryStruct {
	var fdbs []fdbIPStruct
	var macs []*FdbEntryStruct
	var fs *FdbEntryStruct

	CP, err := nlink.ReadFDB(ctx)
	if err != nil || len(CP) == 3 {
		return macs
	}

	CPs := strings.Split(CP[2:len(CP)-3], "},{")
	for i := 0; i < len(CPs); i++ {
		var fi fdbIPStruct
		err := json.Unmarshal([]byte(fmt.Sprintf("{%v}", CPs[i])), &fi)
		if err != nil {
			log.Printf("netlink: error-%v", err)
		}
		fdbs = append(fdbs, fi)
	}
	for _, m := range fdbs {
		fs = ParseFdb(m)
		if preFilterMac(fs) {
			macs = append(macs, fs)
		}
	}
	return macs
}

// lookupRoute check the routes
func lookupRoute(dst net.IP, v *infradb.Vrf) (*RouteStruct, bool) {
	// FIXME: If the semantic is to return the current entry of the NetlinkDB
	//  routing table, a direct lookup in Linux should only be done as fallback
	//  if there is no match in the DB.
	var CP string
	var err error
	if v.Spec.Vni != nil {
		CP, err = nlink.RouteLookup(ctx, dst.String(), path.Base(v.Name))
	} else {
		CP, err = nlink.RouteLookup(ctx, dst.String(), "")
	}
	if err != nil {
		log.Printf("netlink : Command error %v\n", err)
		return &RouteStruct{}, false
	}
	r := cmdProcessRt(v, CP, int(*v.Metadata.RoutingTable[0]))
	log.Printf("netlink: %+v\n", r)
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
			G, _ := infradb.GetVrf("//network.opiproject.org/vrfs/GRD")
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

// installFilterNH install the neighbor filter
func installFilterNH(nh *NexthopStruct) bool {
	check := checkNhType(nh.NhType)
	keep := check && nh.Resolved && len(nh.RouteRefs) != 0
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

// installFilterFDB install fdb filer
func installFilterFDB(fdb *FdbEntryStruct) bool {
	// Drop entries w/o VLAN ID or associated LogicalBridge ...
	// ... other than with L2 nexthops of type VXLAN and BridgePort ...
	// ... and VXLAN entries with unresolved underlay nextop.
	keep := fdb.VlanID != 0 && fdb.lb != nil && checkFdbType(fdb.Type) && fdb.Nexthop.Resolved
	if !keep {
		log.Printf("netlink: install_filter: dropping {%v}", fdb)
	}
	return keep
}

// installFilterL2N install the l2 filter
func installFilterL2N(l2n *L2NexthopStruct) bool {
	keep := !(l2n.Type == 0 && l2n.Resolved && len(l2n.FdbRefs) == 0)
	if !keep {
		log.Printf("netlink: install_filter fDB: dropping {%+v}", l2n)
	}
	return keep
}

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

// notifyDBChanges notify the database changes
func notifyDBChanges() {
	var routeEventStr = make([]string, 0)
	var nexthopEventStr = make([]string, 0)
	var fdbEventStr = make([]string, 0)
	var l2nexthopEventStr = make([]string, 0)

	for _, s := range notifyEvents {
		routeEventStr = append(routeEventStr, "route"+s)
		nexthopEventStr = append(nexthopEventStr, "nexthop"+s)
		fdbEventStr = append(fdbEventStr, "fdb_entry"+s)
		l2nexthopEventStr = append(l2nexthopEventStr, "l2_nexthop"+s)
	}
	type NlDBCopy struct {
		RDB   map[routeKey]*RouteStruct
		NDB   map[nexthopKey]*NexthopStruct
		FBDB  map[fdbKey]*FdbEntryStruct
		L2NDB map[l2NexthopKey]*L2NexthopStruct
	}
	latestdb := NlDBCopy{RDB: latestRoutes, NDB: latestNexthop, FBDB: latestFDB, L2NDB: latestL2Nexthop}
	olddb := NlDBCopy{RDB: routes, NDB: Nexthops, FBDB: fDB, L2NDB: l2Nexthops}
	var eventStr []interface{}
	eventStr = append(eventStr, routeEventStr)
	eventStr = append(eventStr, nexthopEventStr)
	eventStr = append(eventStr, fdbEventStr)
	eventStr = append(eventStr, l2nexthopEventStr)
	// routes
	oldgenmap = make(map[interface{}]interface{})
	latestgenmap = make(map[interface{}]interface{})
	for k, v := range latestdb.RDB {
		latestgenmap[k] = v
	}
	for k, v := range olddb.RDB {
		oldgenmap[k] = v
	}
	notify_changes(latestgenmap, oldgenmap, eventStr[0].([]string))
	// Nexthops
	oldgenmap = make(map[interface{}]interface{})
	latestgenmap = make(map[interface{}]interface{})
	for k, v := range latestdb.NDB {
		latestgenmap[k] = v
	}
	for k, v := range olddb.NDB {
		oldgenmap[k] = v
	}
	notify_changes(latestgenmap, oldgenmap, eventStr[1].([]string))
	// fDB
	oldgenmap = make(map[interface{}]interface{})
	latestgenmap = make(map[interface{}]interface{})
	for k, v := range latestdb.FBDB {
		latestgenmap[k] = v
	}
	for k, v := range olddb.FBDB {
		oldgenmap[k] = v
	}
	notify_changes(latestgenmap, oldgenmap, eventStr[2].([]string))
	// L2Nexthop
	oldgenmap = make(map[interface{}]interface{})
	latestgenmap = make(map[interface{}]interface{})
	for k, v := range latestdb.L2NDB {
		latestgenmap[k] = v
	}
	for k, v := range olddb.L2NDB {
		oldgenmap[k] = v
	}
	notify_changes(latestgenmap, oldgenmap, eventStr[3].([]string))
}

// resyncWithKernel fun resyncs with kernal db
func resyncWithKernel() {
	// Build a new DB snapshot from netlink and other sources
	readLatestNetlinkState()
	// Annotate the latest DB entries
	annotateDBEntries()
	// Filter the latest DB to retain only entries to be installed
	applyInstallFilters()
	// Compute changes between current and latest DB versions and inform subscribers about the changes
	notifyDBChanges()
	routes = latestRoutes
	Nexthops = latestNexthop
	Neighbors = latestNeighbors
	fDB = latestFDB
	l2Nexthops = latestL2Nexthop
	DeleteLatestDB()
}

// DeleteLatestDB deletes the latest db snap
func DeleteLatestDB() {
	latestRoutes = make(map[routeKey]*RouteStruct)
	latestNeighbors = make(map[neighKey]neighStruct)
	latestNexthop = make(map[nexthopKey]*NexthopStruct)
	latestFDB = make(map[fdbKey]*FdbEntryStruct)
	latestL2Nexthop = make(map[l2NexthopKey]*L2NexthopStruct)
}

// monitorNetlink moniters the netlink
func monitorNetlink() {
	for !stopMonitoring.Load().(bool) {
		log.Printf("netlink: Polling netlink databases.")
		resyncWithKernel()
		log.Printf("netlink: Polling netlink databases completed.")
		time.Sleep(time.Duration(pollInterval) * time.Second)
	}
	log.Printf("netlink: Stopped periodic polling. Waiting for Infra DB cleanup to finish")
	time.Sleep(2 * time.Second)
	log.Printf("netlink: One final netlink poll to identify what's still left.")
	resyncWithKernel()
	// Inform subscribers to delete configuration for any still remaining Netlink DB objects.
	log.Printf("netlink: Delete any residual objects in DB")
	for _, r := range routes {
		notifyAddDel(r, "RouteDeleted")
	}
	for _, nexthop := range Nexthops {
		notifyAddDel(nexthop, "NexthopDeleted")
	}
	for _, m := range fDB {
		notifyAddDel(m, "fdb_entry_deleted")
	}
	log.Printf("netlink: DB cleanup completed.")
}

// Initialize function intializes config
func Initialize() {
	pollInterval = config.GlobalConfig.Netlink.PollInterval
	log.Printf("netlink: poll interval: %v", pollInterval)
	nlEnabled := config.GlobalConfig.Netlink.Enabled
	if !nlEnabled {
		log.Printf("netlink: netlink_monitor disabled")
		return
	}
	for i := 0; i < len(config.GlobalConfig.Netlink.PhyPorts); i++ {
		phyPorts[config.GlobalConfig.Netlink.PhyPorts[i].Name] = config.GlobalConfig.Netlink.PhyPorts[i].Vsi
	}
	getlink()
	ctx = context.Background()
	nlink = utils.NewNetlinkWrapperWithArgs(false)
	// stopMonitoring = false
	stopMonitoring.Store(false)
	go monitorNetlink() // monitor Thread started
}

// DeInitialize function handles stops functionality
func DeInitialize() {
	// stopMonitoring = true
	stopMonitoring.Store(true)
}
