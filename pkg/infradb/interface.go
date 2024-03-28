// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.
// Copyright (c) 2022-2023 Intel Corporation, or its subsidiaries.
// Copyright (C) 2023 Nordix Foundation.

package infradb

import (
	"strconv"
	"time"
)

// EvpnObject is an interface for all domain objects in evpn-gw
type EvpnObject[T any] interface {
	ToPb() T
	GetName() string
}

func generateVersion() string {
	timestampMicroseconds := time.Now().UTC().UnixNano() / int64(time.Microsecond)
	return strconv.FormatInt(timestampMicroseconds, 10)
}
