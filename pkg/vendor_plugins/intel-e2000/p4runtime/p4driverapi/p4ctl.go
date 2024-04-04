// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

// Package p4driverapi handles p4 driver realted functionality
package p4driverapi

import (
	"bytes"
	"context"
	"encoding/binary"

	// "encoding/hex"
	"fmt"
	"net"

	// "strings"
	"log"
	"time"

	logr "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	p4_v1 "github.com/p4lang/p4runtime/go/p4/v1"

	"github.com/antoninbas/p4runtime-go-client/pkg/client"
	"github.com/antoninbas/p4runtime-go-client/pkg/signals"
)

const (
	defaultDeviceID = 1
	lpmStr          = "lpm"
	ternaryStr      = "ternary"
)

var (
	// Ctx var of type context
	Ctx context.Context

	// P4RtC var of \p4 runtime client
	P4RtC *client.Client
)

// TableEntry p4 table entry type
type TableEntry struct {
	Tablename string
	TableField
	Action
}

// Action p4 table action type
type Action struct {
	ActionName string
	Params     []interface{}
}

// TableField p4 table field type
type TableField struct {
	FieldValue map[string][2]interface{}
	Priority   int32
}

// uint16toBytes convert uint16 to bytes
func uint16toBytes(val uint16) []byte {
	return []byte{byte(val >> 8), byte(val)}
}

// boolToBytes convert bool to bytes
func boolToBytes(val bool) []byte {
	if val {
		return []byte{1}
	}
	return []byte{0}
}

// uint32toBytes convert uint32 to bytes
func uint32toBytes(num uint32) []byte {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, num)
	return bytes
}

// Buildmfs builds the match fields
func Buildmfs(tablefield TableField) (map[string]client.MatchInterface, bool, error) {
	var isTernary bool
	isTernary = false
	mfs := map[string]client.MatchInterface{}
	for key, value := range tablefield.FieldValue {
		switch v := value[0].(type) {
		case net.HardwareAddr:
			mfs[key] = &client.ExactMatch{Value: value[0].(net.HardwareAddr)}
		case uint16:
			// if value[1].(string) == lpmStr {
			switch value[1].(string) {
			case lpmStr:
				mfs[key] = &client.LpmMatch{Value: uint16toBytes(value[0].(uint16)), PLen: 31}
			// } else if value[1].(string) == ternaryStr {
			case ternaryStr:
				isTernary = true
				mfs[key] = &client.TernaryMatch{Value: uint16toBytes(value[0].(uint16)), Mask: uint32toBytes(4294967295)}
			// } else {
			default:
				mfs[key] = &client.ExactMatch{Value: uint16toBytes(value[0].(uint16))}
			}
		case *net.IPNet:
			maskSize, _ := v.Mask.Size()
			ip := v.IP.To4()
			// if value[1].(string) == lpmStr {
			switch value[1].(string) {
			case lpmStr:
				mfs[key] = &client.LpmMatch{Value: v.IP.To4(), PLen: int32(maskSize)}
			// } else if value[1].(string) == ternaryStr {
			case ternaryStr:
				isTernary = true
				mfs[key] = &client.TernaryMatch{Value: []byte(ip), Mask: uint32toBytes(4294967295)}
			// } else {
			default:
				mfs[key] = &client.ExactMatch{Value: []byte(ip)}
			}
		case net.IP:

			switch value[1].(string) {
			case lpmStr:

				mfs[key] = &client.LpmMatch{Value: value[0].(net.IP).To4(), PLen: 24}
			// } else if value[1].(string) == ternaryStr {
			case ternaryStr:
				isTernary = true
				mfs[key] = &client.TernaryMatch{Value: []byte(v), Mask: uint32toBytes(4294967295)}
			// } else {
			default:
				mfs[key] = &client.ExactMatch{Value: []byte(v)}
			}
		case bool:
			mfs[key] = &client.ExactMatch{Value: boolToBytes(value[0].(bool))}
		case uint32:
			switch value[1].(string) {
			case lpmStr:

				mfs[key] = &client.LpmMatch{Value: uint32toBytes(value[0].(uint32)), PLen: 31}
			// } else if value[1].(string) == ternaryStr {
			case ternaryStr:
				isTernary = true
				mfs[key] = &client.TernaryMatch{Value: uint32toBytes(value[0].(uint32)), Mask: uint32toBytes(4294967295)}
			// } else {
			default:
				mfs[key] = &client.ExactMatch{Value: uint32toBytes(value[0].(uint32))}
			}
		default:
			log.Println("intel-e2000: Unknown field ", v)
			return mfs, false, fmt.Errorf("invalid inputtype %d for %s", v, key)
		}
	}
	return mfs, isTernary, nil
}

// GetEntry get the entry
func GetEntry(table string) ([]*p4_v1.TableEntry, error) {
	entry, err1 := P4RtC.ReadTableEntryWildcard(Ctx, table)
	return entry, err1
}

// DelEntry deletes the entry
func DelEntry(entry TableEntry) error {
	Options := &client.TableEntryOptions{
		Priority: entry.TableField.Priority,
	}
	mfs, isTernary, err := Buildmfs(entry.TableField)
	if err != nil {
		log.Fatalf("intel-e2000: Error in Building mfs: %v", err)
		return err
	}
	if isTernary {
		entry := P4RtC.NewTableEntry(entry.Tablename, mfs, nil, Options)
		return P4RtC.DeleteTableEntry(Ctx, entry)
	}

	entryP := P4RtC.NewTableEntry(entry.Tablename, mfs, nil, nil)
	return P4RtC.DeleteTableEntry(Ctx, entryP)
}

/*// mustMarshal marshal the msg
func mustMarshal(msg proto.Message) []byte {
	data, err := proto.Marshal(msg)
	if err != nil {
		panic(err) // You should handle errors appropriately in your code
	}
	return data
}*/

// AddEntry adds an entry
func AddEntry(entry TableEntry) error {
	Options := &client.TableEntryOptions{
		Priority: entry.TableField.Priority,
	}
	mfs, isTernary, err := Buildmfs(entry.TableField)
	if err != nil {
		log.Fatalf("intel-e2000: Error in Building mfs: %v", err)
		return err
	}
	params := make([][]byte, len(entry.Action.Params))
	for i := 0; i < len(entry.Action.Params); i++ {
		switch v := entry.Action.Params[i].(type) {
		case uint16:
			buf := new(bytes.Buffer)
			err1 := binary.Write(buf, binary.BigEndian, v)
			if err1 != nil {
				log.Println("intel-e2000: binary.Write failed:", err1)
				return err1
			}
			params[i] = buf.Bytes()
		case uint32:
			buf := new(bytes.Buffer)
			err1 := binary.Write(buf, binary.BigEndian, v)
			if err1 != nil {
				log.Println("inte-e2000: binary.Write failed:", err1)
				return err1
			}
			params[i] = buf.Bytes()
		case net.HardwareAddr:
			params[i] = v
		case net.IP:
			params[i] = v
		default:
			log.Println("intel-e2000: Unknown actionparam", v)
			return nil
		}
	}

	actionSet := P4RtC.NewTableActionDirect(entry.Action.ActionName, params)

	if isTernary {
		entryP := P4RtC.NewTableEntry(entry.Tablename, mfs, actionSet, Options)
		return P4RtC.InsertTableEntry(Ctx, entryP)
	}
	entryP := P4RtC.NewTableEntry(entry.Tablename, mfs, actionSet, nil)
	return P4RtC.InsertTableEntry(Ctx, entryP)
}

/*// encodeMac encodes the mac from string
func encodeMac(macAddrString string) []byte {
	str := strings.Replace(macAddrString, ":", "", -1)
	decoded, _ := hex.DecodeString(str)
	return decoded
}*/

// NewP4RuntimeClient get the p4 runtime client
func NewP4RuntimeClient(binPath string, p4infoPath string, conn *grpc.ClientConn) error {
	Ctx = context.Background()
	c := p4_v1.NewP4RuntimeClient(conn)
	resp, err := c.Capabilities(Ctx, &p4_v1.CapabilitiesRequest{})
	if err != nil {
		logr.Fatalf("intel-e2000: Error in Capabilities RPC: %v", err)
		return err
	}
	logr.Infof("intel-e2000: P4Runtime server version is %s", resp.P4RuntimeApiVersion)

	stopCh := signals.RegisterSignalHandlers()

	electionID := &p4_v1.Uint128{High: 0, Low: 1}

	P4RtC = client.NewClient(c, defaultDeviceID, electionID)
	arbitrationCh := make(chan bool)

	errs := make(chan error, 1)
	go func() {
		errs <- P4RtC.Run(stopCh, arbitrationCh, nil)
	}()

	waitCh := make(chan struct{})

	go func() {
		sent := false
		for isPrimary := range arbitrationCh {
			if isPrimary {
				logr.Infof("We are the primary client!")
				if !sent {
					waitCh <- struct{}{}
					sent = true
				}
			} else {
				logr.Infof("We are not the primary client!")
			}
		}
	}()

	func() {
		timeout := 5 * time.Second
		Ctx2, cancel := context.WithTimeout(Ctx, timeout)
		defer cancel()
		select {
		case <-Ctx2.Done():
			logr.Fatalf("Could not become the primary client within %v", timeout)
		case <-errs:
			logr.Fatalf("Could not get the client within %v", timeout)
		case <-waitCh:
		}
	}()
	logr.Info("Setting forwarding pipe")
	if _, err := P4RtC.SetFwdPipe(Ctx, binPath, p4infoPath, 0); err != nil {
		logr.Fatalf("Error when setting forwarding pipe: %v", err)
		return err
	}
	return nil
}
