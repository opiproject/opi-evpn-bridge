package netlink

import (
	"context"
	"fmt"
	"net"
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
var stopMonitoring atomic.Value

// strNone variable
var strNone = "NONE"

var zebraStr = "zebra"

// l2nexthopID
var l2nexthopID = 16

// nhNextID Variable
var nhNextID = 16

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

//--------------------------------------------------------------------------
//###  NexthopStruct Database Entries
//--------------------------------------------------------------------------

// l2NhIDCache
var l2NhIDCache = make(map[l2NexthopKey]int)

// nhIDCache Variable
var nhIDCache = make(map[nexthopKey]int)

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

// RouteOperations add, update, delete
var RouteOperations = Operations{Add: RouteAdded, Update: RouteUpdated, Delete: RouteDeleted}

// NexthopOperations add, update, delete
var NexthopOperations = Operations{Add: NexthopAdded, Update: NexthopUpdated, Delete: NexthopDeleted}

// FdbOperations add, update, delete
var FdbOperations = Operations{Add: FdbEntryAdded, Update: FdbEntryUpdated, Delete: FdbEntryDeleted}

// L2NexthopOperations add, update, delete
var L2NexthopOperations = Operations{Add: L2NexthopAdded, Update: L2NexthopUpdated, Delete: L2NexthopDeleted}

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
