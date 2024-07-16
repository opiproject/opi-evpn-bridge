package netlink

import (
	"encoding/json"
	"log"

	"github.com/opiproject/opi-evpn-bridge/pkg/infradb"
)

// ParseFdb parse the fdb
func ParseFdb(fdbIP FdbIPStruct) *FdbEntryStruct {
	var fdbentry FdbEntryStruct
	fdbentry.VlanID = fdbIP.Vlan
	fdbentry.Mac = fdbIP.Mac
	fdbentry.Key = FdbKey{fdbIP.Vlan, fdbIP.Mac}
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
	dev := fdbIP.Ifname
	dst := fdbIP.Dst
	fdbentry.Nexthop.ParseL2NH(fdbentry.VlanID, dev, dst, fdbentry.lb, fdbentry.bp)
	fdbentry.Type = fdbentry.Nexthop.Type
	return &fdbentry
}

// preFilterMac filter the mac
func preFilterMac(f *FdbEntryStruct) bool {
	// TODO m.nexthop.dst
	if f.VlanID != 0 || (f.Nexthop.Dst != nil && !f.Nexthop.Dst.IsUnspecified()) {
		return true
	}
	return false
}

// readFDB read the fdb from db
func readFDB() []*FdbEntryStruct {
	var fdbs []FdbIPStruct
	var macs []*FdbEntryStruct
	var fs *FdbEntryStruct

	cp, err := nlink.ReadFDB(ctx)
	if err != nil || len(cp) <= 3 {
		return macs
	}

	var rawMessages []json.RawMessage
	err = json.Unmarshal([]byte(cp), &rawMessages)
	if err != nil {
		log.Printf("netlink fdb: JSON unmarshal error: %v %v : %v\n", err, cp, rawMessages)
		return macs
	}
	cps := make([]string, 0, len(rawMessages))
	for _, rawMsg := range rawMessages {
		cps = append(cps, string(rawMsg))
	}
	for i := 0; i < len(cps); i++ {
		var fi FdbIPStruct
		err := json.Unmarshal([]byte(cps[i]), &fi)
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

// addFdbEntry add fdb entries
func addFdbEntry(m *FdbEntryStruct) {
	m = addL2Nexthop(m)
	// TODO
	// logger.debug(f"Adding {m.format()}.")
	latestFDB[m.Key] = m
}

// annotate the route
func (fdb *FdbEntryStruct) annotate() {
	if fdb.VlanID == 0 && fdb.lb != nil {
		return
	}

	fdb.Metadata = make(map[interface{}]interface{})
	l2n := fdb.Nexthop
	if l2n != nil {
		fdb.Metadata["nh_id"] = l2n.ID
		if l2n.Type == VXLAN {
			fdbEntry := latestFDB[FdbKey{None, fdb.Mac}]
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
}

func checkFdbType(fdbtype int) bool {
	var portType = map[int]struct{}{BRIDGEPORT: {}, VXLAN: {}}
	if _, ok := portType[fdbtype]; ok {
		return true
	}
	return false
}

// installFilterFDB install fdb filer
func installFilterFDB(fdb *FdbEntryStruct) bool {
	// Drop entries w/o VLAN ID or associated LogicalBridge ...
	// ... other than with L2 nexthops of type VXLAN and BridgePort ...
	// ... and VXLAN entries with unresolved underlay nextop.
	keep := fdb.VlanID != 0 && fdb.lb != nil && checkFdbType(fdb.Type) && fdb.Nexthop.Resolved
	return keep
}

func (fdb *FdbEntryStruct) deepEqual(fdbOld *FdbEntryStruct, nc bool) bool {
	if fdb.VlanID != fdbOld.VlanID || fdb.Mac != fdbOld.Mac || fdb.Key != fdbOld.Key || fdb.Type != fdbOld.Type {
		return false
	}
	if nc {
		if fdb.Nexthop != nil || fdbOld != nil {
			ret := fdb.Nexthop.deepEqual(fdbOld.Nexthop, false)
			if !ret {
				return false
			}
		}
	}
	return true
}
