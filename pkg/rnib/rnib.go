// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package rnib

import (
	"context"

	"github.com/gogo/protobuf/proto"
	"github.com/onosproject/onos-lib-go/pkg/logging"

	"github.com/onosproject/onos-lib-go/pkg/errors"

	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	toposdk "github.com/onosproject/onos-ric-sdk-go/pkg/topo"
)

var log = logging.GetLogger()

func NewClient() (Client, error) {
	sdkClient, err := toposdk.NewClient()
	if err != nil {
		return Client{}, err
	}
	cl := Client{
		client: sdkClient,
	}
	return cl, nil
}

// TopoClient R-NIB client interface
type TopoClient interface {
	WatchE2Connections(ctx context.Context, ch chan topoapi.Event) error
	GetE2NodeList(ctx context.Context) ([]topoapi.ID, error)
	GetRanconfigurationStructures(ctx context.Context, nodeID topoapi.ID) ([]*topoapi.RanconfigurationStructure, error)
	GetE2NodeAspects(ctx context.Context, nodeID topoapi.ID) (*topoapi.E2Node, error)
	SetPolicyRatiosList(ctx context.Context, nodeID topoapi.ID, msg *topoapi.ConfigurationStructureList) error
	AddConfigurationStructureItem(ctx context.Context, nodeID topoapi.ID, msg *topoapi.ConfigurationStructure) error
	UpdateConfigurationStructureItem(ctx context.Context, nodeID topoapi.ID, msg *topoapi.ConfigurationStructure) error
}

// Client topo SDK client
type Client struct {
	client toposdk.Client
}

func (t *Client) WatchE2Connections(ctx context.Context, ch chan topoapi.Event) error {
	err := t.client.Watch(ctx, ch, toposdk.WithWatchFilters(getControlRelationFilter()))
	if err != nil {
		return err
	}
	return nil
}

func getControlRelationFilter() *topoapi.Filters {
	filter := &topoapi.Filters{
		KindFilter: &topoapi.Filter{
			Filter: &topoapi.Filter_Equal_{
				Equal_: &topoapi.EqualFilter{
					Value: topoapi.E2NodeKind,
				},
			},
		},
	}
	return filter
}

func (t *Client) GetE2NodeList(ctx context.Context) ([]topoapi.ID, error) {
	objects, err := t.client.List(ctx, toposdk.WithListFilters(getControlRelationFilter()))
	if err != nil {
		return nil, err
	}

	e2Nodes := make([]topoapi.ID, 0)
	for _, object := range objects {
		e2Nodes = append(e2Nodes, object.ID)
	}

	return e2Nodes, nil
}

func (t *Client) GetE2NodeAspects(ctx context.Context, nodeID topoapi.ID) (*topoapi.E2Node, error) {
	object, err := t.client.Get(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	e2Node := &topoapi.E2Node{}
	err = object.GetAspect(e2Node)
	if err != nil {
		return nil, err
	}

	return e2Node, nil
}

func (t *Client) HasCCCRANFunction(ctx context.Context, nodeID topoapi.ID, oid string) bool {
	log.Debugf("nodeID: %v", nodeID)
	e2Node, err := t.GetE2NodeAspects(ctx, nodeID)
	if err != nil {
		log.Warn(err)
		return false
	}

	log.Debugf("e2Node: %v", e2Node)
	for _, sm := range e2Node.GetServiceModels() {
		log.Debugf("sm: %v", sm)
		if sm.OID == oid {
			return true
		}
	}
	return false
}

func (t *Client) GetRanconfigurationStructures(ctx context.Context, nodeID topoapi.ID) ([]*topoapi.RanconfigurationStructure, error) {
	result := make([]*topoapi.RanconfigurationStructure, 0)
	e2Node, err := t.GetE2NodeAspects(ctx, nodeID)
	log.Debugf("GetRanconfigurationStructures: GetE2NodeAspects: %v", e2Node)
	if err != nil {
		return nil, err
	}

	for smName, sm := range e2Node.GetServiceModels() {
		for _, ranFunc := range sm.GetRanFunctions() {
			cccRanFunc := &topoapi.CCCRanFunction{}
			err = proto.Unmarshal(ranFunc.GetValue(), cccRanFunc)
			if err != nil {
				log.Debugf("RanFunction for SM - %v, URL - %v does not have CCC RAN Function Definition:\n%v", smName, ranFunc.GetTypeUrl(), err)
				continue
			}
			for _, ranStruct := range cccRanFunc.GetRanStructures() {
				if ranStruct.Name == "O-RRMPolicyRatio" {
					cccAttributes := make([]*topoapi.Attribute, 0)
					cccRanStruct := &topoapi.RanconfigurationStructure{
						Name: ranStruct.Name,
					}
					cccAttributes = append(cccAttributes, ranStruct.GetAttribute()...)
					cccRanStruct.Attribute = cccAttributes
					result = append(result, cccRanStruct)
				}
			}
		}
	}
	return result, nil
}

func (t *Client) GetListOfPolicyRatios(ctx context.Context, nodeID topoapi.ID) (*topoapi.ConfigurationStructureList, error) {
	object, err := t.client.Get(ctx, nodeID)
	log.Debugf("GetListOfPolicyRatios: Get: %v", object)
	if err != nil {
		return nil, err
	}

	value := &topoapi.ConfigurationStructureList{}
	err = object.GetAspect(value)
	log.Debugf("GetListOfPolicyRatios: GetAspect: %v", value)
	if err != nil {
		return nil, err
	}

	return value, nil
}

func (t *Client) SetPolicyRatiosList(ctx context.Context, nodeID topoapi.ID, msg *topoapi.ConfigurationStructureList) error {
	object, err := t.client.Get(ctx, nodeID)
	log.Debugf("SetPolicyRatiosList: Get: %v", object)
	if err != nil {
		return err
	}

	err = object.SetAspect(msg)
	if err != nil {
		return err
	}

	log.Debugf("SetPolicyRatiosList: Get: %v", object)
	err = t.client.Update(ctx, object)
	if err != nil {
		return err
	}

	return nil
}

func (t *Client) AddConfigurationStructureItem(ctx context.Context, nodeID topoapi.ID, msg *topoapi.ConfigurationStructure) error {
	policyRatioList, err := t.GetListOfPolicyRatios(ctx, nodeID)
	log.Debugf("AddConfigurationStructureItem: GetListOfPolicyRatios: %v", policyRatioList)
	if err != nil {
		confgStructure := make([]*topoapi.ConfigurationStructure, 0)
		policyRatioList = &topoapi.ConfigurationStructureList{
			ConfigurationStructure: confgStructure,
		}
	}

	for _, policyRatio := range policyRatioList.ConfigurationStructure {
		if policyRatio.ConfigurationName == msg.ConfigurationName {
			err := t.UpdateConfigurationStructureItem(ctx, nodeID, msg)
			if err != nil {
				return err
			}
			return nil
		}
	}

	policyRatioList.ConfigurationStructure = append(policyRatioList.ConfigurationStructure, msg)
	err = t.SetPolicyRatiosList(ctx, nodeID, policyRatioList)
	if err != nil {
		return err
	}
	return nil
}

func (t *Client) UpdateConfigurationStructureItem(ctx context.Context, nodeID topoapi.ID, msg *topoapi.ConfigurationStructure) error {
	err := t.DeleteConfigurationStructureItem(ctx, nodeID, msg)
	log.Debugf("UpdateConfigurationStructureItem: DeleteConfigurationStructureItem: %v", msg)
	if err != nil {
		return err
	}

	err = t.AddConfigurationStructureItem(ctx, nodeID, msg)
	if err != nil {
		return err
	}
	return nil
}

func (t *Client) DeleteConfigurationStructureItem(ctx context.Context, nodeID topoapi.ID, msg *topoapi.ConfigurationStructure) error {
	policyRatioList, err := t.GetListOfPolicyRatios(ctx, nodeID)
	log.Debugf("DeleteConfigurationStructureItem: GetListOfPolicyRatios: %v", policyRatioList)
	if err != nil {
		return errors.NewNotFound("node %v has no policies", nodeID)
	}

	for i := 0; i < len(policyRatioList.GetConfigurationStructure()); i++ {
		if policyRatioList.GetConfigurationStructure()[i].GetConfigurationName() == msg.ConfigurationName {
			policyRatioList.ConfigurationStructure = append(policyRatioList.ConfigurationStructure[:i], policyRatioList.ConfigurationStructure[i+1:]...)
			break
		}
	}

	err = t.SetPolicyRatiosList(ctx, nodeID, policyRatioList)
	if err != nil {
		return err
	}
	return nil
}

var _ TopoClient = &Client{}
