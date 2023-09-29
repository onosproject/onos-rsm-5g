// SPDX-FileCopyrightText: 2020-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package e2

import (
	"fmt"
	e2api "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	"github.com/onosproject/onos-e2-sm/servicemodels/e2sm_ccc/pdubuilder"
	e2sm_ccc "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_ccc/v1/e2sm-ccc-ies"
	e2sm_common_ies "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_ccc/v1/e2sm-common-ies"
	"google.golang.org/protobuf/proto"
)

// NewControlMessageHandler creates the new control message handler
func NewControlMessageHandler() ControlMessageHandler {
	return ControlMessageHandler{}
}

// ControlMessageHandler is a struct to handle control message
type ControlMessageHandler struct {
}

// CreateControlRequest returns the control request message
func (c *ControlMessageHandler) CreateControlRequest(ricStyleType *e2sm_common_ies.RicStyleType, configurationStructure *e2sm_ccc.ConfigurationStructureWrite) (*e2api.ControlMessage, error) {
	hdr, err := c.CreateControlHeader(ricStyleType)
	if err != nil {
		return nil, err
	}

	payload, err := c.CreateControlPayload(ricStyleType, configurationStructure)
	if err != nil {
		return nil, err
	}

	return &e2api.ControlMessage{
		Header:  hdr,
		Payload: payload,
	}, nil
}

// CreateControlHeader creates the control message header
func (c *ControlMessageHandler) CreateControlHeader(ricStyleType *e2sm_common_ies.RicStyleType) ([]byte, error) {
	controlHeaderFormat := &e2sm_ccc.ControlHeaderFormat{
		ControlHeaderFormat: &e2sm_ccc.ControlHeaderFormat_E2SmCccControlHeaderFormat1{
			E2SmCccControlHeaderFormat1: &e2sm_ccc.E2SmCCcControlHeaderFormat1{
				RicStyleType: ricStyleType,
			},
		},
	}

	hdr, err := pdubuilder.CreateE2SmCCcRIcControlHeader(controlHeaderFormat)
	if err != nil {
		return nil, err
	}
	hdrProtoBytes, err := proto.Marshal(hdr)
	if err != nil {
		return nil, err
	}
	return hdrProtoBytes, nil
}

// CreateControlPayload creates the control message payload
func (c *ControlMessageHandler) CreateControlPayload(ricStyleType *e2sm_common_ies.RicStyleType, configurationStructure *e2sm_ccc.ConfigurationStructureWrite) ([]byte, error) {
	var err error
	var controlMessageFormat *e2sm_ccc.ControlMessageFormat
	var msg *e2sm_ccc.E2SmCCcRIcControlMessage
	var msgProtoBytes []byte

	configurationStructureWrite := make([]*e2sm_ccc.ConfigurationStructureWrite, 0)
	configurationStructureWrite = append(configurationStructureWrite, configurationStructure)

	// TODO: GA: To complete creation of message format1 using configurationStructure
	if ricStyleType.GetValue() == 1 {
		controlMessageFormat = &e2sm_ccc.ControlMessageFormat{
			ControlMessageFormat: &e2sm_ccc.ControlMessageFormat_E2SmCccControlMessageFormat1{
				E2SmCccControlMessageFormat1: &e2sm_ccc.E2SmCCcControlMessageFormat1{
					ListOfConfigurationStructures: &e2sm_ccc.ListOfConfigurationStructures{
						Value: configurationStructureWrite,
					},
				},
			},
		}
	} else if ricStyleType.GetValue() == 2 {
		err := fmt.Errorf("ricStyleType type (%d) not currently supported", ricStyleType.GetValue())
		log.Warn(err)
		return nil, err
	} else {
		err := fmt.Errorf("wrong ricStyleType type (%d)", ricStyleType.GetValue())
		log.Error(err)
		return nil, err
	}

	msg, err = pdubuilder.CreateE2SmCCcRIcControlMessage(controlMessageFormat)
	if err != nil {
		return nil, err
	}
	msgProtoBytes, err = proto.Marshal(msg)
	if err != nil {
		return nil, err
	}

	return msgProtoBytes, err
}
