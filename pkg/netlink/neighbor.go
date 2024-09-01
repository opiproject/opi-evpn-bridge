package netlink

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	vn "github.com/vishvananda/netlink"
)

// NeighKey strcture of neighbor
type NeighKey struct {
	Dst     string
	VrfName string
	Dev     int
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

// NhRouteInfo neighbor route info
type NhRouteInfo struct {
	ID       int
	Gateway  string
	Dev      string
	Scope    string
	Protocol string
	Flags    []string
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

// latestNeighbors Variable
var latestNeighbors = make(map[NeighKey]NeighStruct)

// nhIDCache Variable
var nhIDCache = make(map[NexthopKey]int)

// rtNNeighbor
const (
	rtNNeighbor = 1111
)

// preFilterNeighbor pre filter the neighbors
func (neigh NeighStruct) preFilterNeighbor() bool {
	if neigh.Neigh0.State != vn.NUD_NONE && neigh.Neigh0.State != vn.NUD_INCOMPLETE && neigh.Neigh0.State != vn.NUD_FAILED && nameIndex[neigh.Neigh0.LinkIndex] != "lo" {
		return true
	}

	return false
}

// parseNeigh parses the neigh
func parseNeigh(nm []NeighIPStruct, v string) NeighList {
	var nl NeighList
	for _, nd := range nm {
		var ns NeighStruct
		ns.Neigh0.Type = OTHER
		ns.VrfName = v
		if nd.Dev != "" {
			vrf, _ := vn.LinkByName(nd.Dev)
			ns.Neigh0.LinkIndex = vrf.Attrs().Index
		}
		if nd.Dst != "" {
			ipnet := &net.IPNet{
				IP: net.ParseIP(nd.Dst),
			}
			ns.Neigh0.IP = ipnet.IP
		}
		if len(nd.State) != 0 {
			ns.Neigh0.State = getState(nd.State[0])
		}
		if nd.Lladdr != "" {
			ns.Neigh0.HardwareAddr, _ = net.ParseMAC(nd.Lladdr)
		}
		if nd.Protocol != "" {
			ns.Protocol = nd.Protocol
		}
		ns.Key = NeighKey{VrfName: v, Dst: ns.Neigh0.IP.String(), Dev: ns.Neigh0.LinkIndex}
		if ns.preFilterNeighbor() {
			nl.NS = append(nl.NS, ns)
		}
	}
	return nl
}

// cmdProcessNb process the neighbor command
func cmdProcessNb(nbs []NeighIPStruct, v string) NeighList {
	neigh := parseNeigh(nbs, v)
	return neigh
}

// addNeigh adds the neigh
func addNeigh(dump NeighList) {
	for _, n := range dump.NS {
		n = n.neighborAnnotate()
		if len(latestNeighbors) == 0 || !CheckNdup(n.Key) {
			latestNeighbors[n.Key] = n
		}
	}
}

// readNeighbors reads the nighbors
func readNeighbors(v *infradb.Vrf) {
	var n NeighList
	var err error
	var nb string
	var nbs []NeighIPStruct
	if v.Spec.Vni == nil {
		/* No support for "ip neighbor show" command in netlink library Raised ticket https://github.com/vishvananda/netlink/issues/913 ,
		   so using ip command as WA */
		nb, err = nlink.ReadNeigh(ctx, "")
	} else {
		nb, err = nlink.ReadNeigh(ctx, path.Base(v.Name))
	}
	if len(nb) <= 3 || err != nil {
		return
	}
	var rawMessages []json.RawMessage
	err = json.Unmarshal([]byte(nb), &rawMessages)
	if err != nil {
		log.Printf("netlink nb: JSON unmarshal error: %v %v : %v\n", err, nb, rawMessages)
		return
	}
	cps := make([]string, 0, len(rawMessages))
	for _, rawMsg := range rawMessages {
		cps = append(cps, string(rawMsg))
	}
	for i := 0; i < len(cps); i++ {
		var ni NeighIPStruct
		err := json.Unmarshal([]byte(cps[i]), &ni)
		if err != nil {
			log.Println("netlink: error-", err)
		}
		nbs = append(nbs, ni)
	}
	n = cmdProcessNb(nbs, v.Name)
	addNeigh(n)
}

// CheckNdup checks the duplication of neighbor
func CheckNdup(tmpKey NeighKey) bool {
	var dup = false
	for k := range latestNeighbors {
		if k == tmpKey {
			dup = true
			break
		}
	}
	return dup
}

// checkNeigh checks the nighbor
func checkNeigh(nk NeighKey) bool {
	for k := range latestNeighbors {
		if k == nk {
			return true
		}
	}
	return false
}

func (neigh *NeighStruct) printNeigh() string {
	var Proto string
	if neigh == nil {
		return strNone
	}
	if neigh.Protocol == "" {
		Proto = strNone
	} else {
		Proto = neigh.Protocol
	}
	str := fmt.Sprintf("Neighbor(vrf=%s dst=%s lladdr=%s dev=%s proto=%s state=%s) ", neigh.VrfName, neigh.Neigh0.IP.String(), neigh.Neigh0.HardwareAddr.String(), nameIndex[neigh.Neigh0.LinkIndex], Proto, getStateStr(neigh.Neigh0.State))
	return str
}

// nolint
func (neigh NeighStruct) neighborAnnotate() NeighStruct {
	neigh.Metadata = make(map[interface{}]interface{})
	var phyFlag bool
	phyFlag = false
	for k := range phyPorts {
		if nameIndex[neigh.Neigh0.LinkIndex] == k {
			phyFlag = true
		}
	}
	if strings.HasPrefix(nameIndex[neigh.Neigh0.LinkIndex], path.Base(neigh.VrfName)) && neigh.Protocol != zebraStr {
		pattern := fmt.Sprintf(`%s-\d+$`, path.Base(neigh.VrfName))
		mustcompile := regexp.MustCompile(pattern)
		s := mustcompile.FindStringSubmatch(nameIndex[neigh.Neigh0.LinkIndex])
		var lb *infradb.LogicalBridge
		var bp *infradb.BridgePort
		vID := strings.Split(s[0], "-")[1]
		lbs, _ := infradb.GetAllLBs()
		vlanID, err := strconv.ParseUint(vID, 10, 32)
		if err != nil {
			panic(err)
		}
		for _, lB := range lbs {
			if lB.Spec.VlanID == uint32(vlanID) {
				lb = lB
				break
			}
		}
		if lb != nil {
			bP := lb.MacTable[neigh.Neigh0.HardwareAddr.String()]
			if bP != "" {
				bp, _ = infradb.GetBP(bP)
			}
		}
		if bp != nil {
			neigh.Type = SVI
			neigh.Metadata["vport_id"] = bp.Metadata.VPort
			neigh.Metadata["vlanID"] = uint32(vlanID)
			neigh.Metadata["portType"] = bp.Spec.Ptype
		} else {
			neigh.Type = IGNORE
		}
	} else if strings.HasPrefix(nameIndex[neigh.Neigh0.LinkIndex], path.Base(neigh.VrfName)) && neigh.Protocol == zebraStr {
		pattern := fmt.Sprintf(`%s-\d+$`, path.Base(neigh.VrfName))
		mustcompile := regexp.MustCompile(pattern)
		s := mustcompile.FindStringSubmatch(neigh.Dev)
		var lb *infradb.LogicalBridge
		vID := strings.Split(s[0], "-")[1]
		lbs, _ := infradb.GetAllLBs()
		vlanID, err := strconv.ParseUint(vID, 10, 32)
		if err != nil {
			panic(err)
		}
		for _, lB := range lbs {
			if lB.Spec.VlanID == uint32(vlanID) {
				lb = lB
				break
			}
		}
		if lb.Spec.Vni != nil {
			vid, err := strconv.Atoi(vID)
			if err != nil {
				panic(err)
			}
			fdbEntry := latestFDB[FdbKey{vid, neigh.Neigh0.HardwareAddr.String()}]
			neigh.Metadata["l2_nh"] = fdbEntry.Nexthop
			neigh.Type = VXLAN // confirm this later
		}
	} else if path.Base(neigh.VrfName) == "GRD" && phyFlag && neigh.Protocol != zebraStr {
		vrf, _ := infradb.GetVrf("//network.opiproject.org/vrfs/GRD")
		r, ok := lookupRoute(neigh.Neigh0.IP, vrf)
		if ok {
			if r.Nexthops[0].nexthop.LinkIndex == neigh.Neigh0.LinkIndex {
				neigh.Type = PHY
				neigh.Metadata["vport_id"] = phyPorts[nameIndex[neigh.Neigh0.LinkIndex]]
			} else {
				neigh.Type = IGNORE
			}
		} else {
			neigh.Type = OTHER
		}
	}
	return neigh
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
