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

// preFilterNeighbor pre filter the neighbors
func preFilterNeighbor(n NeighStruct) bool {
	if n.Neigh0.State != vn.NUD_NONE && n.Neigh0.State != vn.NUD_INCOMPLETE && n.Neigh0.State != vn.NUD_FAILED && nameIndex[n.Neigh0.LinkIndex] != "lo" {
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
		if preFilterNeighbor(ns) {
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
		n = neighborAnnotate(n)
		if len(latestNeighbors) == 0 {
			latestNeighbors[n.Key] = n
		} else if !CheckNdup(n.Key) {
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
	CPs := make([]string, 0, len(rawMessages))
	for _, rawMsg := range rawMessages {
		CPs = append(CPs, string(rawMsg))
	}
	for i := 0; i < len(CPs); i++ {
		var ni NeighIPStruct
		err := json.Unmarshal([]byte(CPs[i]), &ni)
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

func printNeigh(neigh *NeighStruct) string {
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
func neighborAnnotate(neighbor NeighStruct) NeighStruct {
	neighbor.Metadata = make(map[interface{}]interface{})
	var phyFlag bool
	phyFlag = false
	for k := range phyPorts {
		if nameIndex[neighbor.Neigh0.LinkIndex] == k {
			phyFlag = true
		}
	}
	if strings.HasPrefix(nameIndex[neighbor.Neigh0.LinkIndex], path.Base(neighbor.VrfName)) && neighbor.Protocol != zebraStr {
		pattern := fmt.Sprintf(`%s-\d+$`, path.Base(neighbor.VrfName))
		mustcompile := regexp.MustCompile(pattern)
		s := mustcompile.FindStringSubmatch(nameIndex[neighbor.Neigh0.LinkIndex])
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
			bP := lb.MacTable[neighbor.Neigh0.HardwareAddr.String()]
			if bP != "" {
				bp, _ = infradb.GetBP(bP)
			}
		}
		if bp != nil {
			neighbor.Type = SVI
			neighbor.Metadata["vport_id"] = bp.Metadata.VPort
			neighbor.Metadata["vlanID"] = uint32(vlanID)
			neighbor.Metadata["portType"] = bp.Spec.Ptype
		} else {
			neighbor.Type = IGNORE
		}
	} else if strings.HasPrefix(nameIndex[neighbor.Neigh0.LinkIndex], path.Base(neighbor.VrfName)) && neighbor.Protocol == zebraStr {
		pattern := fmt.Sprintf(`%s-\d+$`, path.Base(neighbor.VrfName))
		mustcompile := regexp.MustCompile(pattern)
		s := mustcompile.FindStringSubmatch(neighbor.Dev)
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
			fdbEntry := latestFDB[FdbKey{vid, neighbor.Neigh0.HardwareAddr.String()}]
			neighbor.Metadata["l2_nh"] = fdbEntry.Nexthop
			neighbor.Type = VXLAN // confirm this later
		}
	} else if path.Base(neighbor.VrfName) == "GRD" && phyFlag && neighbor.Protocol != zebraStr {
		vrf, _ := infradb.GetVrf("//network.opiproject.org/vrfs/GRD")
		r, ok := lookupRoute(neighbor.Neigh0.IP, vrf)
		if ok {
			if r.Nexthops[0].nexthop.LinkIndex == neighbor.Neigh0.LinkIndex {
				neighbor.Type = PHY
				neighbor.Metadata["vport_id"] = phyPorts[nameIndex[neighbor.Neigh0.LinkIndex]]
			} else {
				neighbor.Type = IGNORE
			}
		} else {
			neighbor.Type = OTHER
		}
	}
	return neighbor
}
