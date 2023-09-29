// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package monitoring

import (
	"context"

	e2api "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	e2sm_ccc "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_ccc/v1/e2sm-ccc-ies"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-rsm-5g/pkg/broker"
	appConfig "github.com/onosproject/onos-rsm-5g/pkg/config"
	"github.com/onosproject/onos-rsm-5g/pkg/rnib"
	"google.golang.org/protobuf/proto"
)

var log = logging.GetLogger()

func NewMonitor(opts ...Option) *Monitor {
	options := Options{}
	for _, opt := range opts {
		opt.apply(&options)
	}

	return &Monitor{
		streamReader: options.Monitor.StreamReader,
		appConfig:    options.App.AppConfig,
		nodeID:       options.Monitor.NodeID,
		rnibClient:   options.App.Client,
	}
}

type Monitor struct {
	streamReader broker.StreamReader
	appConfig    *appConfig.AppConfig
	nodeID       topoapi.ID
	rnibClient   rnib.Client
}

func (m *Monitor) Start(ctx context.Context) error {
	errCh := make(chan error)
	go func() {
		for {
			indMsg, err := m.streamReader.Recv(ctx)
			if err != nil {
				errCh <- err
			}
			err = m.processIndication(ctx, indMsg, m.nodeID)
			if err != nil {
				errCh <- err
			}
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Monitor) processIndication(ctx context.Context, indMsg e2api.Indication, nodeID topoapi.ID) error {
	indHeader := e2sm_ccc.E2SmCCcRIcIndicationHeader{}
	indPayload := e2sm_ccc.E2SmCCcRIcIndicationMessage{}

	err := proto.Unmarshal(indMsg.Header, &indHeader)
	if err != nil {
		return err
	}

	err = proto.Unmarshal(indMsg.Payload, &indPayload)
	if err != nil {
		return err
	}

	if indPayload.GetIndicationMessageFormat().GetE2SmCccIndicationMessageFormat1() != nil {
		err = m.processConfigurationsReported(ctx, nodeID, indHeader.GetIndicationHeaderFormat().GetE2SmCccIndicationHeaderFormat1(), indPayload.GetIndicationMessageFormat().GetE2SmCccIndicationMessageFormat1())
		if err != nil {
			return err
		}
	} else if indPayload.GetIndicationMessageFormat().GetE2SmCccIndicationMessageFormat2() != nil {
		err = m.processCellsReported(ctx, nodeID, indHeader.GetIndicationHeaderFormat().GetE2SmCccIndicationHeaderFormat1(), indPayload.GetIndicationMessageFormat().GetE2SmCccIndicationMessageFormat2())
		if err != nil {
			return err
		}
	} else {
		return errors.NewNotSupported("Invalid E2SM-CCC-IndicationMessageFormat")
	}

	return nil
}

func (m *Monitor) processConfigurationsReported(ctx context.Context, nodeID topoapi.ID, indHdr *e2sm_ccc.E2SmCCcIndicationHeaderFormat1, indMsg *e2sm_ccc.E2SmCCcIndicationMessageFormat1) error {
	log.Debugf("Received indication message (Configuration Reported) hdr: %v / msg: %v", indHdr, indMsg)
	for _, confStruct := range indMsg.GetListOfConfigurationStructuresReported().GetValue() {
		log.Debugf("Change Type: %v", confStruct.GetChangeType())
		confStructName := confStruct.GetRanConfigurationStructureName()
		policyRatio := confStruct.GetValuesOfAttributes().GetRanConfigurationStructure().GetORrmpolicyRatio()
		if policyRatio == nil {
			log.Debugf("RanConfigurationStructure %v does not have O-RRMPolicyRatio", confStructName)
			continue
		}
		log.Debugf("PolicyRatio for valuesOfAttributes: %v", policyRatio)

		// Ignoring oldValueOfAttributes for now
		// Later can be used to validate present/current policies
		// oldPolicyRatio := confStruct.GetOldValuesOfAttributes().GetRanConfigurationStructure().GetORrmpolicyRatio()

		sliceMemberList := make([]*topoapi.RrmPolicyMember, 0)

		for _, sliceMember := range policyRatio.RRmpolicyMemberList.Value {
			slice := &topoapi.RrmPolicyMember{
				PlmnId: &topoapi.Plmnidentity{
					Value: sliceMember.GetPlmnId().Value,
				},
				Snssai: &topoapi.SNSsai{
					Sst: &topoapi.Sst{
						Value: sliceMember.Snssai.GetSSt().Value,
					},
					Sd: &topoapi.Sd{
						Value: sliceMember.Snssai.GetSD().Value,
					},
				},
			}
			sliceMemberList = append(sliceMemberList, slice)
		}

		msg := &topoapi.ConfigurationStructure{
			ConfigurationName: string(confStructName.GetValue()),
			PolicyRatio: &topoapi.ORRmpolicyRatio{
				ResourceType:  topoapi.ResourceType(policyRatio.ResourceType),
				SchedulerType: topoapi.SchedulerType(policyRatio.SchedulerType),
				RrmPolicyMemberList: &topoapi.RrmPolicyMemberList{
					RrmPolicyMember: sliceMemberList,
				},
				RrmPolicyMaxRatio:       policyRatio.RRmpolicyMaxRatio,
				RrmPolicyMinRatio:       policyRatio.RRmpolicyMinRatio,
				RrmPolicyDedicatedRatio: policyRatio.RRmpolicyDedicatedRatio,
			},
		}

		switch confStruct.GetChangeType() {
			case e2sm_ccc.ChangeType_CHANGE_TYPE_ADDITION:
			msg.ChangeType = topoapi.ChangeType_CHANGE_TYPE_ADDITION
			log.Debugf("AddConfigurationStructureItem to %v: %v", nodeID, msg)
			err := m.rnibClient.AddConfigurationStructureItem(ctx, nodeID, msg)
			if err != nil {
				return err
			}

		case e2sm_ccc.ChangeType_CHANGE_TYPE_MODIFICATION:
			msg.ChangeType = topoapi.ChangeType_CHANGE_TYPE_MODIFICATION
			log.Debugf("UpdateConfigurationStructureItem to %v: %v", nodeID, msg)
			err := m.rnibClient.UpdateConfigurationStructureItem(ctx, nodeID, msg)
			if err != nil {
				return err
			}

		case e2sm_ccc.ChangeType_CHANGE_TYPE_DELETION:
			msg.ChangeType = topoapi.ChangeType_CHANGE_TYPE_DELETION
			log.Debugf("DeleteConfigurationStructureItem to %v: %v", nodeID, msg)
			err := m.rnibClient.DeleteConfigurationStructureItem(ctx, nodeID, msg)
			if err != nil {
				return err
			}

		default:
			log.Debugf("Ignoraing Change Type: %v", confStruct.GetChangeType())
		}
	}
	return nil
}

func (m *Monitor) processCellsReported(ctx context.Context, nodeID topoapi.ID, indHdr *e2sm_ccc.E2SmCCcIndicationHeaderFormat1, indMsg *e2sm_ccc.E2SmCCcIndicationMessageFormat2) error {
	log.Debugf("Received indication message (Cells Reported) hdr: %v / msg: %v", indHdr, indMsg)
	log.Warnf("Currently not supporting Cell-level messages")
	return nil
}
