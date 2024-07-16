package netlink

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"sync/atomic"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	eb "github.com/opiproject/opi-evpn-bridge/pkg/netlink/eventbus"
	"github.com/opiproject/opi-evpn-bridge/pkg/utils"
	vn "github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
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
var stopMonitoring atomic.Bool

// strNone variable
var strNone = "NONE"

var zebraStr = "zebra"

// l2nexthopID
var l2nexthopID = 16

// nhNextID Variable
var nhNextID = 16

// routes Variable
var routes = make(map[RouteKey]*RouteStruct)

// Nexthops Variable
var nexthops = make(map[NexthopKey]*NexthopStruct)

// fDB Variable
var fDB = make(map[FdbKey]*FdbEntryStruct)

// l2Nexthops Variable
var l2Nexthops = make(map[L2NexthopKey]*L2NexthopStruct)

// latestRoutes Variable
var latestRoutes = make(map[RouteKey]*RouteStruct)

// latestNexthop Variable
var latestNexthop = make(map[NexthopKey]*NexthopStruct)

// latestNeighbors Variable
var latestNeighbors = make(map[NeighKey]NeighStruct)

// latestFDB Variable
var latestFDB = make(map[FdbKey]*FdbEntryStruct)

// latestL2Nexthop Variable
var latestL2Nexthop = make(map[L2NexthopKey]*L2NexthopStruct)

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
var nameIndex = make(map[int]string)

// oldgenmap old map
var oldgenmap = make(map[interface{}]interface{})

// latestgenmap latest map
var latestgenmap = make(map[interface{}]interface{})

//--------------------------------------------------------------------------
//###  NexthopStruct Database Entries
//--------------------------------------------------------------------------

// l2NhIDCache
var l2NhIDCache = make(map[L2NexthopKey]int)

// nhIDCache Variable
var nhIDCache = make(map[NexthopKey]int)

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

// Event Types
const (
	ROUTE = iota
	NEXTHOP
	FDB
	L2NEXTHOP
)

// rtNNeighbor
const (
	rtNNeighbor = 1111
)

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

// routeOperations add, update, delete
var routeOperations = Operations{Add: RouteAdded, Update: RouteUpdated, Delete: RouteDeleted}

// nexthopOperations add, update, delete
var nexthopOperations = Operations{Add: NexthopAdded, Update: NexthopUpdated, Delete: NexthopDeleted}

// fdbOperations add, update, delete
var fdbOperations = Operations{Add: FdbEntryAdded, Update: FdbEntryUpdated, Delete: FdbEntryDeleted}

// l2NexthopOperations add, update, delete
var l2NexthopOperations = Operations{Add: L2NexthopAdded, Update: L2NexthopUpdated, Delete: L2NexthopDeleted}

// Operations Structure
type Operations struct {
	Add    string
	Update string
	Delete string
}

// Event Structure
type Event struct {
	EventType int
	Operation Operations
}

// NeighKey strcture of neighbor
type NeighKey struct {
	Dst     string
	VrfName string
	Dev     int
}

// RouteKey structure of route description
type RouteKey struct {
	Table int
	Dst   string
}

// NexthopKey structure of nexthop
type NexthopKey struct {
	VrfName string
	Dst     string
	Dev     int
	Local   bool
}

// FdbKey structure key for sorting theFDB entries
type FdbKey struct {
	VlanID int
	Mac    string
}

// L2NexthopKey is l2 neighbor key
type L2NexthopKey struct {
	Dev    string
	VlanID int
	Dst    string
}

// NeighIPStruct nighbor ip structure
type NeighIPStruct struct {
	Dst         string
	Dev         string
	Lladdr      string
	ExternLearn string
	State       []string
	Protocol    string
}

// FdbIPStruct fdb ip structure
type FdbIPStruct struct {
	Mac    string
	Ifname string
	Vlan   int
	Flags  []string
	Master string
	State  string
	Dst    string
}

// NhRouteInfo neighbor route info
type NhRouteInfo struct {
	ID       int
	Gateway  string
	Dev      string
	Scope    string
	Protocol string
	Flags    []string
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
	Key      L2NexthopKey
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
	Key      FdbKey
	State    string
	lb       *infradb.LogicalBridge
	bp       *infradb.BridgePort
	Nexthop  *L2NexthopStruct
	Type     int
	Metadata map[interface{}]interface{}
	Err      error
}

// NeighStruct structure
type NeighStruct struct {
	Neigh0   vn.Neigh
	Protocol string
	VrfName  string
	Type     int
	Dev      string
	Err      error
	Key      NeighKey
	Metadata map[interface{}]interface{}
}

// NeighList structure
type NeighList struct {
	NS []NeighStruct
}

// VrfStatusGetter gets vrf status
type VrfStatusGetter interface {
	GetVrfOperStatus() infradb.VrfOperStatus
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

// getFlagString return flag of type string
func getFlagString(flag int) string {
	str, ok := testFlag[flag]
	if !ok {
		return ""
	}
	return str
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

// dumpRouteDB dump the route database
func dumpRouteDB() string {
	var s string
	log.Printf("netlink: Dump Route table:\n")
	s = "Route table:\n"
	for _, n := range latestRoutes {
		var via string
		if n.Route0.Gw == nil {
			via = strNone
		} else {
			via = n.Route0.Gw.String()
		}
		str := fmt.Sprintf("Route(vrf=%s dst=%s type=%s proto=%s metric=%d  via=%s dev=%s nhid= %d Table= %d)", n.Vrf.Name, n.Route0.Dst.String(), n.NlType, getProto(n), n.Route0.Priority, via, nameIndex[n.Route0.LinkIndex], n.Nexthops[0].ID, n.Route0.Table)
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
		str := fmt.Sprintf("Nexthop(id=%d vrf=%s dst=%s dev=%s Local=%t weight=%d flags=[%s] #routes=%d Resolved=%t neighbor=%s) ", n.ID, n.Vrf.Name, n.nexthop.Gw.String(), nameIndex[n.nexthop.LinkIndex], n.Local, n.Weight, getFlagString(n.nexthop.Flags), len(n.RouteRefs), n.Resolved, printNeigh(n.Neighbor))
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
		str := fmt.Sprintf("Neighbor(vrf=%s dst=%s lladdr=%s dev=%s proto=%s state=%s Type : %d) ", n.VrfName, n.Neigh0.IP.String(), n.Neigh0.HardwareAddr.String(), nameIndex[n.Neigh0.LinkIndex], Proto, getStateStr(n.Neigh0.State), n.Type)
		log.Println(str)
		s += str
		s += "\n"
	}
	s += "\n\n"
	return s
}

// checkProto checks the proto type
func checkProto(proto int) bool {
	var protos = map[int]struct{}{unix.RTPROT_BOOT: {}, unix.RTPROT_STATIC: {}, 196: {}}
	if _, ok := protos[proto]; ok {
		return true
	}
	return false
}

func processAndnotify[K comparable, V any](latestDB, oldDB map[K]V, eventType int, ops Operations) {
	oldgenmap = make(map[interface{}]interface{})
	latestgenmap = make(map[interface{}]interface{})
	for k, v := range latestDB {
		latestgenmap[k] = v
	}
	for k, v := range oldDB {
		oldgenmap[k] = v
	}
	event := Event{
		EventType: eventType,
		Operation: ops,
	}
	notify_changes(latestgenmap, oldgenmap, event)
}

// nolint
func notify_changes(new_db map[interface{}]interface{}, old_db map[interface{}]interface{}, event Event) {
	db2 := old_db
	db1 := new_db
	/* Checking the Updated entries in the netlink db by comparing the individual keys and their corresponding values in old and new db copies
	   entries with same keys with different values and send the notification to vendor specific module */
	for k1, v1 := range db1 {
		v2, ok := db2[k1]
		if !ok {
			continue
		}
		if !deepCheck(v1, v2) {
			// To Avoid in-correct update notification due to race condition in which metadata is nil in new entry and crashing in dcgw module
			if event.EventType == ROUTE || event.EventType == NEXTHOP {
				var status VrfStatusGetter
				var ok bool
				status, ok = v1.(VrfStatusGetter)
				if !ok {
					log.Printf("Netlink: Invalid Type")
					continue
				}
				if status.GetVrfOperStatus() == infradb.VrfOperStatusToBeDeleted {
					notifyAddDel(status, event.Operation.Delete)
					delete(new_db, k1)
					delete(old_db, k1)
					continue
				}
			}
			notifyAddDel(v1, event.Operation.Update)
		}
		delete(new_db, k1)
		delete(old_db, k1)
	}
	for _, r := range new_db { // Added entries notification cases
		notifyAddDel(r, event.Operation.Add)
	}
	for _, r := range old_db { // Deleted entires notification cases
		notifyAddDel(r, event.Operation.Delete)
	}
}

func deepCheck(v1 interface{}, v2 interface{}) bool {
	if reflect.TypeOf(v1) != reflect.TypeOf(v2) {
		return true
	}
	switch t := v1.(type) {
	case *RouteStruct:
		return t.deepEqual(v2.(*RouteStruct), true)
	case *NexthopStruct:
		return t.deepEqual(v2.(*NexthopStruct), true)
	case *FdbEntryStruct:
		return t.deepEqual(v2.(*FdbEntryStruct), true)
	case *L2NexthopStruct:
		return t.deepEqual(v2.(*L2NexthopStruct), true)
	default:
		return true
	}
}

func notifyAddDel(r interface{}, event string) {
	log.Printf("netlink: Notify event: %s\n", event)
	EventBus.Publish(event, r)
}
