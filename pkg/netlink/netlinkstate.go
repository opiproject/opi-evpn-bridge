package netlink

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"reflect"
	"strconv"
	"strings"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
	vn "github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// readLatestNetlinkState reads the latest netlink state
func readLatestNetlinkState() {
	vrfs, _ := infradb.GetAllVrfs()
	for _, v := range vrfs {
		readNeighbors(v) // viswanantha library
		readRoutes(v)    // Viswantha library
	}
	m := readFDB()
	for i := 0; i < len(m); i++ {
		addFdbEntry(m[i])
	}
	dumpDBs()
}

// readNeighbors reads the nighbors
func readNeighbors(v *infradb.Vrf) {
	var N neighList
	var err error
	var Nb string
	var nbs []neighIPStruct
	if v.Spec.Vni == nil {
		/* No support for "ip neighbor show" command in netlink library Raised ticket https://github.com/vishvananda/netlink/issues/913 ,
		   so using ip command as WA */
		Nb, err = nlink.ReadNeigh(ctx, "")
	} else {
		Nb, err = nlink.ReadNeigh(ctx, path.Base(v.Name))
	}
	if len(Nb) <= 3 && err == nil {
		return
	}
	var rawMessages []json.RawMessage
	err = json.Unmarshal([]byte(Nb), &rawMessages)
	if err != nil {
		log.Printf("JSON unmarshal error: %v", err)
		return
	}
	CPs := make([]string, 0, len(rawMessages))
	for _, rawMsg := range rawMessages {
		CPs = append(CPs, string(rawMsg))
	}
	for i := 0; i < len(CPs); i++ {
		var ni neighIPStruct
		err := json.Unmarshal([]byte(CPs[i]), &ni)
		if err != nil {
			log.Println("netlink: error-", err)
		}
		nbs = append(nbs, ni)
	}
	N = cmdProcessNb(nbs, v.Name)
	addNeigh(N)
}

// cmdProcessNb process the neighbor command
func cmdProcessNb(nbs []neighIPStruct, v string) neighList {
	Neigh := parseNeigh(nbs, v)
	return Neigh
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

// readRoutes reads the routes
func readRoutes(v *infradb.Vrf) {
	readRouteFromIP(v)
}

// readRouteFromIP reads the routes from ip
func readRouteFromIP(v *infradb.Vrf) {
	var Rl routeList
	var rm []routeCmdInfo
	var Rt1 int
	var RouteData []routeCmdInfo
	for _, routeSt := range v.Metadata.RoutingTable {
		Rt1 = int(*routeSt)
		Raw, err := nlink.ReadRoute(ctx, strconv.Itoa(Rt1))
		if err != nil || len(Raw) <= 3 {
			log.Printf("netlink: Err Command route\n")
			return
		}
		var rawMessages []json.RawMessage
		err = json.Unmarshal([]byte(Raw), &rawMessages)
		if err != nil {
			log.Printf("JSON unmarshal error: %v", err)
			return
		}
		CPs := make([]string, 0, len(rawMessages))
		for _, rawMsg := range rawMessages {
			CPs = append(CPs, string(rawMsg))
		}
		for i := 0; i < len(CPs); i++ {
			var ri routeCmdInfo
			err := json.Unmarshal([]byte(CPs[i]), &ri)
			if err != nil {
				log.Println("error-", err)
				return
			}
			RouteData = append(RouteData, ri)
		}
		Rl = cmdProcessRt(v, RouteData, Rt1)
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

// cmdProcessRt process the route command
func cmdProcessRt(v *infradb.Vrf, routeData []routeCmdInfo, t int) routeList {
	route := ParseRoute(v, routeData, t)
	return route
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

// checkNhDB checks the neighbor database
func checkNhDB(nhKey nexthopKey) bool {
	for k := range latestNexthop {
		if k == nhKey {
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

// checkNeigh checks the nighbor
func checkNeigh(nk neighKey) bool {
	for k := range latestNeighbors {
		if k == nk {
			return true
		}
	}
	return false
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

// readFDB read the fdb from db
func readFDB() []*FdbEntryStruct {
	var fdbs []fdbIPStruct
	var macs []*FdbEntryStruct
	var fs *FdbEntryStruct

	CP, err := nlink.ReadFDB(ctx)
	if err != nil || len(CP) == 3 {
		return macs
	}

	var rawMessages []json.RawMessage
	err = json.Unmarshal([]byte(CP), &rawMessages)
	if err != nil {
		log.Printf("JSON unmarshal error: %v", err)
		return macs
	}
	CPs := make([]string, 0, len(rawMessages))
	for _, rawMsg := range rawMessages {
		CPs = append(CPs, string(rawMsg))
	}
	for i := 0; i < len(CPs); i++ {
		var fi fdbIPStruct
		err := json.Unmarshal([]byte(CPs[i]), &fi)
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
