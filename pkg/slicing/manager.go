// SPDX-FileCopyrightText: 2020-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package slicing

import (
	"context"
	"fmt"
	"time"

	cccapi "github.com/onosproject/onos-api/go/onos/ccc"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	e2sm_ccc "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_ccc/v1/e2sm-ccc-ies"
	e2sm_common_ies "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_ccc/v1/e2sm-common-ies"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-rsm-5g/pkg/northbound"
	"github.com/onosproject/onos-rsm-5g/pkg/rnib"
	"github.com/onosproject/onos-rsm-5g/pkg/southbound/e2"
)

var log = logging.GetLogger()

type Manager struct {
	cccMsgCh              chan *northbound.CccMsg
	ctrlReqChsSliceUpdate map[string]chan *e2.CtrlMsg
	rnibClient            rnib.Client
	ctrlMsgHandler        e2.ControlMessageHandler
	ackTimer              int
}

func NewManager(opts ...Option) Manager {
	log.Info("Init RSM Slicing Manager for 5G")
	options := Options{}

	for _, opt := range opts {
		opt.apply(&options)
	}

	return Manager{
		cccMsgCh:              options.Chans.CccMsgCh,
		ctrlReqChsSliceUpdate: options.Chans.CtrlReqChsSliceUpdate,
		rnibClient:            options.App.RnibClient,
		ctrlMsgHandler:        e2.NewControlMessageHandler(),
		ackTimer:              options.App.AckTimer,
	}
}

func (m *Manager) Run(ctx context.Context) {
	go m.DispatchNbiMsg(ctx)
}

func (m *Manager) DispatchNbiMsg(ctx context.Context) {
	log.Info("Run nbi msg dispatcher")
	for msg := range m.cccMsgCh {
		log.Debugf("Received message from NBI: %v", msg)
		var ack northbound.Ack
		var err error
		switch msg.Message.(type) {
		case *cccapi.UpdateSliceRequest:
			err = m.handleNbiUpdateSliceRequest(ctx, msg.Message.(*cccapi.UpdateSliceRequest), msg.NodeID)
		default:
			err = fmt.Errorf("unknown msg type: %v", msg)
		}
		if err != nil {
			ack = northbound.Ack{
				Success: false,
				Reason:  err.Error(),
			}
		} else {
			ack = northbound.Ack{
				Success: true,
			}
		}
		msg.AckCh <- ack
	}
}

func (m *Manager) HandleA1UpdateRequest(ctx context.Context, req *topoapi.ConfigurationStructure) error {
	log.Infof("A1 request to Update Slice: %v", req)

	e2Nodes, err := m.rnibClient.GetE2NodeList(ctx)

	if err != nil {
		log.Error(err)
	}

	if len(e2Nodes) == 0 {
		return fmt.Errorf("there are no E2 nodes in onos-topo")
	}

	log.Infof("E2Nodes from topo: %v", e2Nodes)

	for _, nodeID := range e2Nodes {
		cccLoadedCfgs, err := m.rnibClient.GetListOfPolicyRatios(ctx, nodeID)
		if err != nil {
			log.Warn(err)
			return err
		}
		log.Debugf("CCC currently configured O-RRMPolicyRatio(s): %v", cccLoadedCfgs)

		// Making sure that the CCC service model includes/supports "O-RRMPolicyRatio"
		if len(cccLoadedCfgs.GetConfigurationStructure()) == 0 {
			continue
		}

		err = m.rnibClient.UpdateConfigurationStructureItem(ctx, nodeID, req)
		if err != nil {
			return err
		}

		// TODO: GA: For now only handling "Node-level" configuration and control
		// TODO: GA: To update and get this from the `onos.topo.E2Node`
		// aspects, err := m.rnibClient.GetE2NodeAspects(ctx, nodeID)
		// ricStyle, err := aspects.GetServiceModels()
		// 1 => Node Configuration and Control
		// 2 => Cell Configuration and Control
		var ricStyleTy int32 = 1

		ricStyleType := &e2sm_common_ies.RicStyleType{
			Value: ricStyleTy,
		}

		rrmPolicyMemberList := make([]*e2sm_ccc.RrmPolicyMember, 0)
		for _, policyMember := range req.GetPolicyRatio().GetRrmPolicyMemberList().RrmPolicyMember {
			member := &e2sm_ccc.RrmPolicyMember{
				PlmnId: &e2sm_common_ies.Plmnidentity{
					Value: policyMember.PlmnId.Value,
				},
				Snssai: &e2sm_common_ies.SNSsai{
					SSt: &e2sm_common_ies.Sst{
						Value: policyMember.GetSnssai().GetSst().Value,
					},
					SD: &e2sm_common_ies.Sd{
						Value: policyMember.GetSnssai().GetSd().Value,
					},
				},
			}

			rrmPolicyMemberList = append(rrmPolicyMemberList, member)
		}

		rrmPolicyRatioConfig := &e2sm_ccc.ORRmpolicyRatio{
			ResourceType:  e2sm_ccc.ResourceType(req.GetPolicyRatio().GetResourceType()),
			SchedulerType: e2sm_ccc.SchedulerType(req.GetPolicyRatio().GetSchedulerType()),
			RRmpolicyMemberList: &e2sm_ccc.RrmPolicyMemberList{
				Value: rrmPolicyMemberList,
			},
			RRmpolicyMaxRatio:       req.GetPolicyRatio().GetRrmPolicyMaxRatio(),
			RRmpolicyMinRatio:       req.GetPolicyRatio().GetRrmPolicyMinRatio(),
			RRmpolicyDedicatedRatio: req.GetPolicyRatio().GetRrmPolicyDedicatedRatio(),
		}
		log.Warnf("PolicyRatio: %v", rrmPolicyRatioConfig)

		// TODO: GA: Update policy ratio with actual old values
		oldRrmPolicyRatioConfig := &e2sm_ccc.ORRmpolicyRatio{
			ResourceType:  e2sm_ccc.ResourceType(req.GetPolicyRatio().GetResourceType()),
			SchedulerType: e2sm_ccc.SchedulerType(req.GetPolicyRatio().GetSchedulerType()),
			RRmpolicyMemberList: &e2sm_ccc.RrmPolicyMemberList{
				Value: rrmPolicyMemberList,
			},
			RRmpolicyMaxRatio:       req.GetPolicyRatio().GetRrmPolicyMaxRatio(),
			RRmpolicyMinRatio:       req.GetPolicyRatio().GetRrmPolicyMinRatio(),
			RRmpolicyDedicatedRatio: req.GetPolicyRatio().GetRrmPolicyDedicatedRatio(),
		}
		log.Warnf("PolicyRatio: %v", oldRrmPolicyRatioConfig)

		configurationStructure := &e2sm_ccc.ConfigurationStructureWrite{
			RanConfigurationStructureName: &e2sm_ccc.RanConfigurationStructureName{
				Value: []byte(req.ConfigurationName),
			},
			NewValuesOfAttributes: &e2sm_ccc.ValuesOfAttributes{
				RanConfigurationStructure: &e2sm_ccc.RanConfigurationStructure{
					RanConfigurationStructure: &e2sm_ccc.RanConfigurationStructure_ORrmpolicyRatio{
						ORrmpolicyRatio: rrmPolicyRatioConfig,
					},
				},
			},
			OldValuesOfAttributes: &e2sm_ccc.ValuesOfAttributes{
				RanConfigurationStructure: &e2sm_ccc.RanConfigurationStructure{
					RanConfigurationStructure: &e2sm_ccc.RanConfigurationStructure_ORrmpolicyRatio{
						ORrmpolicyRatio: oldRrmPolicyRatioConfig,
					},
				},
			},
		}

		ctrlMsg, err := m.ctrlMsgHandler.CreateControlRequest(ricStyleType, configurationStructure)
		if err != nil {
			return fmt.Errorf("failed to create the control message - %v", err.Error())
		}

		log.Info("Control message: %v", ctrlMsg)
		// send control message
		ackCh := make(chan e2.Ack)
		msg := &e2.CtrlMsg{
			CtrlMsg: ctrlMsg,
			AckCh:   ackCh,
		}
		go func() {
			m.ctrlReqChsSliceUpdate[string(nodeID)] <- msg
		}()

		// ackTimer -1 is for uenib/topo debugging and integration test
		if m.ackTimer != -1 {
			var ack e2.Ack
			select {
			case <-time.After(time.Duration(m.ackTimer) * time.Second):
				return fmt.Errorf("timeout happens: E2 SBI could not send ACK until timer expired")
			case ack = <-ackCh:
			}

			if !ack.Success {
				return fmt.Errorf("%v", ack.Reason)
			}
		}
	}

	return nil
}

func (m *Manager) handleNbiUpdateSliceRequest(ctx context.Context, req *cccapi.UpdateSliceRequest, nodeID topoapi.ID) error {
	log.Infof("Called Update Slice: %v", req)
	// TODO: GA: For now only handling "Node-level" configuration and control
	// TODO: GA: To update and get this from the `onos.topo.E2Node`
	// 1 => Node Configuration and Control
	// 2 => Cell Configuration and Control
	// var ricStyleTy int32 = 1

	// ricStyleType := &e2sm_common_ies.RicStyleType{
	// 	Value: ricStyleTy,
	// }

	// var schedulerType e2sm_ccc.SchedulerType
	// switch req.RrmPolicyRatio.SchedulerType {
	// case cccapi.SchedulerType_SCHEDULER_TYPE_ROUND_ROBIN:
	// 	schedulerType = e2sm_ccc.SchedulerType_SCHEDULER_TYPE_ROUND_ROBIN
	// case cccapi.SchedulerType_SCHEDULER_TYPE_PROPORTIONALLY_FAIR:
	// 	schedulerType = e2sm_ccc.SchedulerType_SCHEDULER_TYPE_PROPORTIONALLY_FAIR
	// case cccapi.SchedulerType_SCHEDULER_TYPE_QOS_BASED:
	// 	schedulerType = e2sm_ccc.SchedulerType_SCHEDULER_TYPE_QOS_BASED
	// default:
	// 	schedulerType = e2sm_ccc.SchedulerType_SCHEDULER_TYPE_ROUND_ROBIN
	// }

	// var resourceType e2sm_ccc.ResourceType
	// switch req.RrmPolicyRatio.ResourceType {
	// case cccapi.ResourceType_RESOURCE_TYPE_PRB_DL:
	// 	resourceType = e2sm_ccc.ResourceType_RESOURCE_TYPE_PRB_DL
	// case cccapi.ResourceType_RESOURCE_TYPE_PRB_UL:
	// 	resourceType = e2sm_ccc.ResourceType_RESOURCE_TYPE_PRB_UL
	// case cccapi.ResourceType_RESOURCE_TYPE_DRB:
	// 	resourceType = e2sm_ccc.ResourceType_RESOURCE_TYPE_DRB
	// case cccapi.ResourceType_RESOURCE_TYPE_RRC:
	// 	resourceType = e2sm_ccc.ResourceType_RESOURCE_TYPE_RRC
	// default:
	// 	resourceType = e2sm_ccc.ResourceType_RESOURCE_TYPE_DRB
	// }

	// rrmPolicyMemberList := make([]*e2sm_ccc.RrmPolicyMember, 0)
	// for _, policyMember := range req.RrmPolicyRatio.GetRrmPolicyMemberList().RrmPolicyMember {
	// 	member := &e2sm_ccc.RrmPolicyMember {
	// 		PlmnId: &e2sm_common_ies.Plmnidentity {
	// 			Value: policyMember.PlmnId.Value,
	// 		},
	// 		Snssai: &e2sm_common_ies.SNSsai{
	// 			SSt: &e2sm_common_ies.Sst {
	// 				Value: policyMember.GetSnssai().GetSst().Value,
	// 			},
	// 			SD: &e2sm_common_ies.Sd {
	// 				Value: policyMember.GetSnssai().GetSd().Value,
	// 			},
	// 		},
	// 	}

	// 	rrmPolicyMemberList = append(rrmPolicyMemberList, member)
	// }

	// // hasSliceItem := m.rnibClient.HasRsmSliceItemAspect(ctx, topoapi.ID(req.E2NodeId), req.SliceId, req.GetSliceType())
	// // if !hasSliceItem {
	// // 	return fmt.Errorf("no slice ID %v in node %v", sliceID, nodeID)
	// // }

	// rrmPolicyRatioConfig := &e2sm_ccc.ORRmpolicyRatio{
	// 	ResourceType:  resourceType,
	// 	SchedulerType: schedulerType,
	// 	RRmpolicyMemberList: &e2sm_ccc.RrmPolicyMemberList{
	// 		Value: rrmPolicyMemberList,
	// 	},
	// 	RRmpolicyMaxRatio:       req.RrmPolicyRatio.RrmPolicyMaxRatio,
	// 	RRmpolicyMinRatio:       req.RrmPolicyRatio.RrmPolicyMinRatio,
	// 	RRmpolicyDedicatedRatio: req.RrmPolicyRatio.RrmPolicyDedicatedRatio,
	// }
	// ctrlMsg, err := m.ctrlMsgHandler.CreateControlRequest(ricStyleType, configurationStructure)
	// if err != nil {
	// 	return fmt.Errorf("failed to create the control message - %v", err.Error())
	// }

	// // send control message
	// ackCh := make(chan e2.Ack)
	// msg := &e2.CtrlMsg{
	// 	CtrlMsg: ctrlMsg,
	// 	AckCh:   ackCh,
	// }
	// go func() {
	// 	m.ctrlReqChsSliceUpdate[string(nodeID)] <- msg
	// }()

	// // ackTimer -1 is for uenib/topo debugging and integration test
	// if m.ackTimer != -1 {
	// 	var ack e2.Ack
	// 	select {
	// 	case <-time.After(time.Duration(m.ackTimer) * time.Second):
	// 		return fmt.Errorf("timeout happens: E2 SBI could not send ACK until timer expired")
	// 	case ack = <-ackCh:
	// 	}

	// 	if !ack.Success {
	// 		return fmt.Errorf("%v", ack.Reason)
	// 	}
	// }

	// sliceAspect, err := m.rnibClient.GetRsmSliceItemAspect(ctx, topoapi.ID(req.E2NodeId), req.SliceId, req.GetSliceType())
	// if err != nil {
	// 	return fmt.Errorf("failed to get slice aspect - slice ID %v in node %v: err: %v", sliceID, nodeID, err)
	// }

	// value := &topoapi.RSMSlicingItem{
	// 	ID:        req.SliceId,
	// 	SliceDesc: "Slice created by onos-rsm-5g xAPP",
	// 	SliceParameters: &topoapi.RSMSliceParameters{
	// 		SchedulerType: topoapi.RSMSchedulerType(req.SchedulerType),
	// 		Weight:        weight,
	// 	},
	// 	SliceType: topoapi.RSMSliceType(req.SliceType),
	// }

	// err = m.rnibClient.UpdateRsmSliceItemAspect(ctx, topoapi.ID(req.E2NodeId), value)
	// if err != nil {
	// 	return fmt.Errorf("failed to update slice information to onos-topo although control message was sent: %v", err)
	// }

	return nil
}
