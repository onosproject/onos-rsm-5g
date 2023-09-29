// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package northbound

import topoapi "github.com/onosproject/onos-api/go/onos/topo"

type Ack struct {
	Success bool
	Reason  string
}

type CccMsg struct {
	NodeID  topoapi.ID
	Message interface{}
	AckCh   chan Ack
}
