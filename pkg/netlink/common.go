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

// eventBus variable
var eventBus = eb.NewEventBus()

// pollInterval variable
var pollInterval int

// dbphyPortslock variable
var phyPorts = make(map[string]int)

// stopMonitoring variable
var stopMonitoring atomic.Bool

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

// NameIndex netlink library var
var nameIndex = make(map[int]string)

// oldgenmap old map
var oldgenmap = make(map[interface{}]interface{})

// latestgenmap latest map
var latestgenmap = make(map[interface{}]interface{})

const (
	strNone  = "NONE"
	zebraStr = "zebra"
)

// Event Types
const (
	ROUTE = iota
	NEXTHOP
	FDB
	L2NEXTHOP
)

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

// checkProto checks the proto type
func checkProto(proto int) bool {
	var protos = map[int]struct{}{unix.RTPROT_BOOT: {}, unix.RTPROT_STATIC: {}, 196: {}}
	if _, ok := protos[proto]; ok {
		return true
	}
	return false
}

func notifyDBCompChanges[K comparable, V any](latestDB, oldDB map[K]V, eventType int, ops Operations) {
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
		log.Printf("netlink: Error Unknown types %T and %T are passed\n", v1, v2)
		return true
	}
}

func notifyAddDel(r interface{}, event string) {
	log.Printf("netlink: Notify event: %s\n", event)
	eventBus.Publish(event, r)
}
