// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package e2

import (
	"context"
	"fmt"
	"strings"

	prototypes "github.com/gogo/protobuf/types"
	e2api "github.com/onosproject/onos-api/go/onos/e2t/e2/v1beta1"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-e2-sm/servicemodels/e2sm_ccc/pdubuilder"
	e2sm_ccc "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_ccc/v1/e2sm-ccc-ies"
	e2sm_common "github.com/onosproject/onos-e2-sm/servicemodels/e2sm_ccc/v1/e2sm-common-ies"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	e2client "github.com/onosproject/onos-ric-sdk-go/pkg/e2/v1beta1"
	"github.com/onosproject/onos-rsm-5g/pkg/broker"
	appConfig "github.com/onosproject/onos-rsm-5g/pkg/config"
	"github.com/onosproject/onos-rsm-5g/pkg/monitoring"
	"github.com/onosproject/onos-rsm-5g/pkg/rnib"
	"google.golang.org/protobuf/proto"
)

var log = logging.GetLogger()

const (
	oid = "1.3.6.1.4.1.53148.1.1.2.103"
)

type Node interface {
	Start() error
	Stop() error
}

type Manager struct {
	appID                 string
	e2Client              e2client.Client
	rnibClient            rnib.Client
	serviceModel          ServiceModelOptions
	appConfig             *appConfig.AppConfig
	streams               broker.Broker
	ctrlReqChsSliceUpdate map[string]chan *CtrlMsg
}

func NewManager(opts ...Option) (Manager, error) {
	log.Info("Init E2 Manager")
	options := Options{}

	for _, opt := range opts {
		opt.apply(&options)
	}

	serviceModelName := e2client.ServiceModelName(options.ServiceModel.Name)
	serviceModelVersion := e2client.ServiceModelVersion(options.ServiceModel.Version)
	appID := e2client.AppID(options.App.AppID)
	e2Client := e2client.NewClient(
		e2client.WithServiceModel(serviceModelName, serviceModelVersion),
		e2client.WithAppID(appID),
		e2client.WithE2TAddress(options.E2TService.Host, options.E2TService.Port))

		rnibClient, err := rnib.NewClient()
		if err != nil {
			return Manager{}, err
		}

		return Manager{
		appID:       options.App.AppID,
		e2Client:    e2Client,
		rnibClient:  rnibClient,
		serviceModel: ServiceModelOptions{
			Name:    options.ServiceModel.Name,
			Version: options.ServiceModel.Version,
		},
		appConfig:             options.App.AppConfig,
		streams:               options.App.Broker,
		ctrlReqChsSliceUpdate: options.App.CtrlReqChsSliceUpdate,
	}, nil
}

func (m *Manager) Start() error {
	log.Info("Start E2 Manager")
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := m.watchE2Connections(ctx)
		if err != nil {
			return
		}
	}()
	return nil
}

func (m *Manager) watchE2Connections(ctx context.Context) error {
	ch := make(chan topoapi.Event)
	err := m.rnibClient.WatchE2Connections(ctx, ch)
	if err != nil {
		log.Warn(err)
		return err
	}

	// creates a new subscription whenever there is a new E2 node connected and supports CCC service model
	for topoEvent := range ch {
		log.Debugf("Received topo event: type %v, message %v", topoEvent.Type, topoEvent)
		e2NodeID := topoEvent.Object.GetID()
		log.Debugf("e2NodeID: %v", e2NodeID)

		switch topoEvent.Type {
		case topoapi.EventType_ADDED, topoapi.EventType_NONE:
			if !m.rnibClient.HasCCCRANFunction(ctx, e2NodeID, oid) {
				log.Debugf("Received topo event does not have CCC RAN function - %v", topoEvent)
				continue
			}

			log.Debugf("New E2NodeID %v connected", e2NodeID)
			cccSupportedCfgs, err := m.rnibClient.GetRanconfigurationStructures(ctx, e2NodeID)

			if err != nil {
				log.Warn(err)
				return err
			}

			log.Debugf("CCC supported configs for O-RRMPolicyRatio: %v", cccSupportedCfgs)

			// Making sure that the CCC service model includes/supports "O-RRMPolicyRatio"
			if len(cccSupportedCfgs) == 0 {
				continue
			}

			// TODO: GA: Using format 3 instead of 1
			// Below commented out code is fully working for format 1. Do not delete it
			eventTriggerFormat3 := &e2sm_ccc.E2SmCCcEventTriggerDefinitionFormat3{
				Period: 1000,
			}
			// eventTriggerFormat1 := &e2sm_ccc.E2SmCCcEventTriggerDefinitionFormat1{}
			// listRanConfStructsForEventTrigger := &e2sm_ccc.ListOfRanconfigurationStructuresForEventTrigger{}
			// var ranConfStructsForEventTrigger []*e2sm_ccc.RanconfigurationStructureForEventTrigger

			actionFormat1 := &e2sm_ccc.E2SmCCcActionDefinitionFormat1{}
			listRanconStructsForAdf := &e2sm_ccc.ListOfRanconfigurationStructuresForAdf{}
			var ranconStructsForAdf []*e2sm_ccc.RanconfigurationStructureForAdf

			// Setting Report type to "change"
			reportType := e2sm_ccc.ReportType(e2sm_ccc.ReportType_REPORT_TYPE_CHANGE)

			for _, cccSupportedCfg := range cccSupportedCfgs {
				ranStructName := &e2sm_ccc.RanConfigurationStructureName{
					Value: []byte(cccSupportedCfg.Name),
				}
				// ranStructCfgName := &e2sm_ccc.RanconfigurationStructureForEventTrigger {
				// 	RanConfigurationStructureName: ranStructName,
				// }
				// ranConfStructsForEventTrigger = append(ranConfStructsForEventTrigger, ranStructCfgName)

				ranStructCfgAdfName := &e2sm_ccc.RanconfigurationStructureForAdf {
					ReportType: reportType,
					RanConfigurationStructureName: ranStructName,
				}
				ranconStructsForAdf = append(ranconStructsForAdf, ranStructCfgAdfName)
			}

			// listRanConfStructsForEventTrigger.Value = ranConfStructsForEventTrigger
			// eventTriggerFormat1.ListOfNodeLevelConfigurationStructuresForEventTrigger = listRanConfStructsForEventTrigger
			// eventTrigger, err := pdubuilder.CreateEventTriggerDefinitionFormatE2SmCccEventTriggerDefinitionFormat1(eventTriggerFormat1)
			eventTrigger, err := pdubuilder.CreateEventTriggerDefinitionFormatE2SmCccEventTriggerDefinitionFormat3(eventTriggerFormat3)
			if err != nil {
				log.Warn(err)
			}

			listRanconStructsForAdf.Value = ranconStructsForAdf
			actionFormat1.ListOfNodeLevelRanconfigurationStructuresForAdf = listRanconStructsForAdf

			actionDefinition, err := pdubuilder.CreateActionDefinitionFormatE2SmCccActionDefinitionFormat1(actionFormat1)
			if err != nil {
				log.Warn(err)
			}
			log.Debugf("GA: eventTrigger: %v", eventTrigger)

			go func(event topoapi.Event) {
				log.Debugf("start creating subscriptions %v", event)
				err := m.newSubscription(ctx, e2NodeID, eventTrigger, actionDefinition)
				if err != nil {
					log.Warn(err)
				}
			}(topoEvent)

		case topoapi.EventType_UPDATED:
			m.ctrlReqChsSliceUpdate[string(e2NodeID)] = make(chan *CtrlMsg)
			go m.watchCtrlSliceUpdated(ctx, e2NodeID)

		case topoapi.EventType_REMOVED:
			log.Debugf("EventType: EventType_REMOVED")
			// Clean up slice information from onos-topo
			// relation := topoEvent.Object.Obj.(*topoapi.Object_Relation)
			// e2NodeID := relation.Relation.TgtEntityID
			// if !m.rnibClient.HasCCCRANFunction(ctx, e2NodeID, oid) {
			// 	log.Debugf("Received topo event does not have CCC RAN function - %v", topoEvent)
			// 	continue
			// }

			// log.Infof("E2 node %v is disconnected", e2NodeID)
			// // Clean up slice information from onos-topo
			// cellIDs, err := m.rnibClient.GetCells(ctx, e2NodeID)
			// if err != nil {
			// 	return err
			// }

			// for _, coi := range cellIDs {
			// 	key := measurements.Key{
			// 		NodeID: string(e2NodeID),
			// 		CellIdentity: measurements.CellIdentity{
			// 			CellID: coi.CellObjectID,
			// 		},
			// 	}
			// 	err = m.measurementStore.Delete(ctx, key)
			// 	if err != nil {
			// 		log.Warn(err)
			// 	}
			// }
		default:
			log.Debugf("EventType: Default/Other (not ADDED, NONE nor REMOVED)")
		}
	}

	return nil
}

func (m *Manager) newSubscription(ctx context.Context, e2NodeID topoapi.ID, eventTrigger *e2sm_ccc.EventTriggerDefinitionFormat, actionDefinition *e2sm_ccc.ActionDefinitionFormat) error {
	err := m.createSubscription(ctx, e2NodeID, eventTrigger, actionDefinition)
	return err
}

func (m *Manager) createSubscription(ctx context.Context, e2nodeID topoapi.ID, eventTrigger *e2sm_ccc.EventTriggerDefinitionFormat, actionDefinition *e2sm_ccc.ActionDefinitionFormat) error {
	log.Info("Creating subscription for E2 node ID with: ", e2nodeID)
	eventTriggerData, err := m.createEventTrigger(eventTrigger)
	if err != nil {
		log.Warn(err)
		return err
	}

	log.Info("actionDefinition: %v", actionDefinition)
	actions, err := m.createSubscriptionActions(actionDefinition)
	if err != nil {
		log.Warn(err)
		return err
	}

	aspects, err := m.rnibClient.GetE2NodeAspects(ctx, e2nodeID)
	if err != nil {
		log.Warn(err)
		return err
	}

	_, err = m.getRanFunction(aspects.ServiceModels)
	if err != nil {
		log.Warn(err)
		return err
	}

	ch := make(chan e2api.Indication)
	node := m.e2Client.Node(e2client.NodeID(e2nodeID))
	// TODO: GA: Testing this
	// subName := fmt.Sprintf("%s-subscription-%s-%v", m.appID, e2nodeID, eventTrigger)
	subName := fmt.Sprintf("%s-subscription", m.appID)
	// End GA
	subSpec := e2api.SubscriptionSpec{
		EventTrigger: e2api.EventTrigger{
			Payload: eventTriggerData,
		},
		Actions: actions,
	}
	log.Debugf("subSpec: %v", subSpec)

	channelID, err := node.Subscribe(ctx, subName, subSpec, ch)
	if err != nil {
		log.Warn(err)
		return err
	}
	log.Debugf("Channel ID:%s", channelID)

	streamReader, err := m.streams.OpenReader(ctx, node, subName, channelID, subSpec)
	if err != nil {
		log.Warn(err)
		return err
	}

	go m.sendIndicationOnStream(streamReader.StreamID(), ch)
	monitor := monitoring.NewMonitor(monitoring.WithAppConfig(m.appConfig),
		monitoring.WithNode(node),
		monitoring.WithNodeID(e2nodeID),
		monitoring.WithStreamReader(streamReader),
		monitoring.WithRNIBClient(m.rnibClient),
		// monitoring.WithRicIndicationTriggerType(eventTrigger),
	)

	err = monitor.Start(ctx)
	if err != nil {
		log.Warn(err)
		return err
	}

	return nil
}

func (m *Manager) createEventTrigger(triggerType *e2sm_ccc.EventTriggerDefinitionFormat) ([]byte, error) {
	eventTriggerDef, err := pdubuilder.CreateE2SmCCcRIceventTriggerDefinition(triggerType)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	protoBytes, err := proto.Marshal(eventTriggerDef)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	return protoBytes, nil
}

func (m *Manager) createSubscriptionActions(actionDefinition *e2sm_ccc.ActionDefinitionFormat) ([]e2api.Action, error) {
	actions := make([]e2api.Action, 0)
	// TODO: GA: To update and get this from the `onos.topo.E2Node`
	// 1 => Node-Level Configuration
	// 2 => Cell-Level Configuration
	var ricStyleType int32 = 1

	ricType := &e2sm_common.RicStyleType{
		Value: ricStyleType,
	}
	actionDefinitionData, err := pdubuilder.CreateE2SmCCcRIcactionDefinition(ricType, actionDefinition)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	protoBytes, err := proto.Marshal(actionDefinitionData)
	if err != nil {
		log.Warn(err)
		return nil, err
	}

	action := &e2api.Action{
		ID:   int32(0),
		Type: e2api.ActionType_ACTION_TYPE_REPORT,

		SubsequentAction: &e2api.SubsequentAction{
			Type:       e2api.SubsequentActionType_SUBSEQUENT_ACTION_TYPE_CONTINUE,
			TimeToWait: e2api.TimeToWait_TIME_TO_WAIT_ZERO,
		},
		Payload: protoBytes,
	}
	actions = append(actions, *action)
	return actions, nil
}

func (m *Manager) getRanFunction(serviceModelsInfo map[string]*topoapi.ServiceModelInfo) (*topoapi.CCCRanFunction, error) {
	for _, sm := range serviceModelsInfo {
		smName := strings.ToLower(sm.Name)
		if smName == string(m.serviceModel.Name) && sm.OID == oid {
			cccRanFunction := &topoapi.CCCRanFunction{}
			for _, ranFunction := range sm.RanFunctions {
				if ranFunction.TypeUrl == ranFunction.GetTypeUrl() {
					err := prototypes.UnmarshalAny(ranFunction, cccRanFunction)
					if err != nil {
						return nil, err
					}
					return cccRanFunction, nil
				}
			}
		}
	}
	return nil, errors.New(errors.NotFound, "cannot retrieve ran functions")
}

func (m *Manager) sendIndicationOnStream(streamID broker.StreamID, ch chan e2api.Indication) {
	streamWriter, err := m.streams.GetWriter(streamID)
	if err != nil {
		log.Error(err)
		return
	}

	for msg := range ch {
		err := streamWriter.Send(msg)
		if err != nil {
			log.Warn(err)
			return
		}
	}
}

func (m *Manager) watchCtrlSliceUpdated(ctx context.Context, e2NodeID topoapi.ID) {
	for ctrlReqMsg := range m.ctrlReqChsSliceUpdate[string(e2NodeID)] {
		log.Debugf("ctrlReqMsg: %v", ctrlReqMsg)
		node := m.e2Client.Node(e2client.NodeID(e2NodeID))
		ctrlRespMsg, err := node.Control(ctx, ctrlReqMsg.CtrlMsg, nil)
		log.Debugf("ctrlRespMsg: %v", ctrlRespMsg)
		if err != nil {
			log.Warnf("Error sending control message - %v", err)
			ack := Ack{
				Success: false,
				Reason:  err.Error(),
			}
			ctrlReqMsg.AckCh <- ack
			continue
		} else if ctrlRespMsg == nil {
			log.Warn(" Ctrl Resp message is nil")
			ack := Ack{
				Success: false,
				Reason:  "Ctrl Resp message is nil",
			}
			ctrlReqMsg.AckCh <- ack
			continue
		}
		ack := Ack{
			Success: true,
		}
		ctrlReqMsg.AckCh <- ack
	}
}

func (m *Manager) Stop() error {
	panic("implement me")
}

var _ Node = &Manager{}
