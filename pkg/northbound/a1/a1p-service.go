// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package a1

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	a1tapi "github.com/onosproject/onos-api/go/onos/a1t/a1"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-rsm-5g/pkg/slicing"
	"github.com/onosproject/onos-lib-go/pkg/logging/service"
	"google.golang.org/grpc"
)

// for testing

var SampleJSON1 = `
{
  "scope": {
    "ueId": "0000000000000001"
  },
  "tspResources": [
    {
      "cellIdList": [
        {"plmnId": {"mcc": "248","mnc": "35"},
          "cId": {"ncI": 39}},
        {"plmnId": {"mcc": "248","mnc": "35"},
         "cId": {"ncI": 40}}
      ],
      "preference": "PREFER"
    },
    {
      "cellIdList": [
        {"plmnId": {"mcc": "248","mnc": "35"},
          "cId": {"ncI": 81}},
        {"plmnId": {"mcc": "248","mnc": "35"},
          "cId": {"ncI": 82}},
        {"plmnId": {"mcc": "248","mnc": "35"},
         "cId": {"ncI": 83}}
      ],
      "preference": "FORBID"
    }
  ]
}
`

var SampleJSON2 = `
{
  "scope": {
    "ueId": "0000000000000002"
  },
  "tspResources": [
    {
      "cellIdList": [
        {"plmnId": {"mcc": "248","mnc": "35"},
          "cId": {"ncI": 39}},
        {"plmnId": {"mcc": "248","mnc": "35"},
         "cId": {"ncI": 40}}
      ],
      "preference": "PREFER"
    },
    {
      "cellIdList": [
        {"plmnId": {"mcc": "248","mnc": "35"},
          "cId": {"ncI": 81}},
        {"plmnId": {"mcc": "248","mnc": "35"},
          "cId": {"ncI": 82}},
        {"plmnId": {"mcc": "248","mnc": "35"},
         "cId": {"ncI": 83}}
      ],
      "preference": "FORBID"
    }
  ]
}
`

var SampleEnforcedStatus = `
{
  "enforceStatus": "ENFORCED"
}
`

var SampleNotEnforcedStatus = `
{
  "enforceStatus": "NOT_ENFORCED",
  "enforceReason": "SCOPE_NOT_APPLICABLE"
}
`

var SampleNotEnforcedPolicyID = "2"

func NewA1PService(slicing slicing.Manager) service.Service {
	log.Infof("A1P service created")
	return &A1PService{slicing: slicing}
}

type A1PService struct {
	slicing slicing.Manager
}

func (a *A1PService) Register(s *grpc.Server) {
	server := &A1PServer{
		sliceManager:    a.slicing,
		TsPolicyTypeMap: make(map[string][]byte),
		StatusUpdateCh:  make(chan *a1tapi.PolicyStatusMessage),
	}
	server.TsPolicyTypeMap["1"] = []byte(SampleJSON1)
	server.TsPolicyTypeMap["2"] = []byte(SampleJSON2)
	a1tapi.RegisterPolicyServiceServer(s, server)
}

type A1PServer struct {
	sliceManager    slicing.Manager
	TsPolicyTypeMap map[string][]byte
	StatusUpdateCh  chan *a1tapi.PolicyStatusMessage
	mu              sync.RWMutex
}

func (a *A1PServer) PolicySetup(ctx context.Context, message *a1tapi.PolicyRequestMessage) (*a1tapi.PolicyResultMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	log.Info("PolicySetup called: %v", message)
	var result map[string]interface{}
	json.Unmarshal(message.Message.Payload, &result)
	log.Infof("Object: %v", result)

	if message.PolicyType.Id != "ORAN_SliceSLATarget_1.0.0" {
		res := &a1tapi.PolicyResultMessage{
			PolicyId:   message.PolicyId,
			PolicyType: message.PolicyType,
			Message: &a1tapi.ResultMessage{
				Header: &a1tapi.Header{
					PayloadType: message.Message.Header.PayloadType,
					RequestId:   message.Message.Header.RequestId,
					Encoding:    message.Message.Header.Encoding,
					AppId:       message.Message.Header.AppId,
				}, Payload: message.Message.Payload,
				Result: &a1tapi.Result{
					Success: false,
					Reason:  "Policy type does not support",
				},
			},
		}
		log.Info("Sending message: %v", res)
		return res, nil
	}

	if _, ok := a.TsPolicyTypeMap[message.PolicyId]; ok {
		res := &a1tapi.PolicyResultMessage{
			PolicyId:   message.PolicyId,
			PolicyType: message.PolicyType,
			Message: &a1tapi.ResultMessage{
				Header: &a1tapi.Header{
					PayloadType: message.Message.Header.PayloadType,
					RequestId:   message.Message.Header.RequestId,
					Encoding:    message.Message.Header.Encoding,
					AppId:       message.Message.Header.AppId,
				}, Payload: message.Message.Payload,
				Result: &a1tapi.Result{
					Success: false,
					Reason:  "Policy ID already exists",
				},
			},
		}
		log.Info("Sending message: %v", res)
		return res, nil
	}

	a.TsPolicyTypeMap[message.PolicyId] = message.Message.Payload

	go func() {
		if message.NotificationDestination != "" {
			statusUpdateMsg := &a1tapi.PolicyStatusMessage{
				PolicyId:   message.PolicyId,
				PolicyType: message.PolicyType,
				Message: &a1tapi.StatusMessage{
					Header: &a1tapi.Header{
						RequestId:   uuid.New().String(),
						AppId:       message.Message.Header.AppId,
						Encoding:    message.Message.Header.Encoding,
						PayloadType: a1tapi.PayloadType_STATUS,
					},
				},
				NotificationDestination: message.NotificationDestination,
			}

			if message.PolicyId == SampleNotEnforcedPolicyID {
				statusUpdateMsg.Message.Payload = []byte(SampleNotEnforcedStatus)
			} else {
				statusUpdateMsg.Message.Payload = []byte(SampleEnforcedStatus)
			}

			a.StatusUpdateCh <- statusUpdateMsg
		}
	}()

	res := &a1tapi.PolicyResultMessage{
		PolicyId:   message.PolicyId,
		PolicyType: message.PolicyType,
		Message: &a1tapi.ResultMessage{
			Header: &a1tapi.Header{
				PayloadType: message.Message.Header.PayloadType,
				RequestId:   message.Message.Header.RequestId,
				Encoding:    message.Message.Header.Encoding,
				AppId:       message.Message.Header.AppId,
			},
			Payload: a.TsPolicyTypeMap[message.PolicyId],
			Result: &a1tapi.Result{
				Success: true,
			},
		},
		NotificationDestination: message.NotificationDestination,
	}
	log.Info("Sending message: %v", res)
	return res, nil
}

func (a *A1PServer) PolicyUpdate(ctx context.Context, message *a1tapi.PolicyRequestMessage) (*a1tapi.PolicyResultMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	log.Info("PolicyUpdate called %v", message)

	var result map[string]interface{}
	json.Unmarshal(message.Message.Payload, &result)
	log.Infof("Object: %v", result)

	if message.PolicyType.Id != "ORAN_SliceSLATarget_1.0.0" {
		res := &a1tapi.PolicyResultMessage{
			PolicyId:   message.PolicyId,
			PolicyType: message.PolicyType,
			Message: &a1tapi.ResultMessage{
				Header: &a1tapi.Header{
					PayloadType: message.Message.Header.PayloadType,
					RequestId:   message.Message.Header.RequestId,
					Encoding:    message.Message.Header.Encoding,
					AppId:       message.Message.Header.AppId,
				}, Payload: message.Message.Payload,
				Result: &a1tapi.Result{
					Success: false,
					Reason:  "Policy type does not support",
				},
			},
		}
		log.Info("Sending message: %v", res)
		return res, nil
	}

	if _, ok := a.TsPolicyTypeMap[message.PolicyId]; !ok {
		res := &a1tapi.PolicyResultMessage{
			PolicyId:   message.PolicyId,
			PolicyType: message.PolicyType,
			Message: &a1tapi.ResultMessage{
				Header: &a1tapi.Header{
					PayloadType: message.Message.Header.PayloadType,
					RequestId:   message.Message.Header.RequestId,
					Encoding:    message.Message.Header.Encoding,
					AppId:       message.Message.Header.AppId,
				}, Payload: message.Message.Payload,
				Result: &a1tapi.Result{
					Success: false,
					Reason:  "Policy ID does not exists",
				},
			},
		}
		log.Info("Sending message: %v", res)
		return res, nil
	}

	// Parsing message received from A1 interface and pass it to R-NIB and send policy update to E2node
	scope, _ := result["scope"].(map[string]interface{})
	sliceSlaObjectives, _ := result["sliceSlaObjectives"].(map[string]interface{})
	resourceType, _ := sliceSlaObjectives["maxNumberOfUes"].(float64)
	schedulerType, _ := sliceSlaObjectives["maxNumberOfPduSessions"].(float64)
	dedicatedRatio, _ := sliceSlaObjectives["maxDlThptPerUe"].(float64)
	minRatio, _ := sliceSlaObjectives["guaDlThptPerSlice"].(float64)
	maxRatio, _ := sliceSlaObjectives["maxDlThptPerSlice"].(float64)
	sliceId, _ := scope["sliceId"].(map[string]interface{})
	plmnId, _ := sliceId["plmnId"].(map[string]interface{})
	mcc, _ := plmnId["mcc"].(string)
	mnc := plmnId["mnc"].(string)
	plmn, err := strconv.ParseUint(mcc + mnc, 10, 32)
	if err != nil {
		log.Warn(err)
	}

	// Pack the uint32 value into a 3-byte []byte slice
	plmnID := make([]byte, 3)
	plmnID[0] = byte(plmn >> 16)
	plmnID[1] = byte(plmn >> 8)
	plmnID[2] = byte(plmn)

	sst := byte(sliceId["sst"].(float64))
	sdString := sliceId["sd"].(string)
	sd, err := hex.DecodeString(sdString)
	if err != nil {
		log.Warn(err)
	}

	sliceMemberList := make([]*topoapi.RrmPolicyMember, 0)

	// There should be a loop here when policy supports multiple slices
	slice := &topoapi.RrmPolicyMember{
		PlmnId: &topoapi.Plmnidentity{
			Value: plmnID,
		},
		Snssai: &topoapi.SNSsai{
			Sst: &topoapi.Sst{
				Value: []byte{sst},
			},
			Sd: &topoapi.Sd{
				Value: sd,
			},
		},
	}
	sliceMemberList = append(sliceMemberList, slice)

	// TODO: GA: Temporarily hardcoding Configuration Name
	confStructName := "Policy_1"

	msg := &topoapi.ConfigurationStructure{
		ChangeType: topoapi.ChangeType_CHANGE_TYPE_MODIFICATION,
		ConfigurationName: string(confStructName),
		PolicyRatio: &topoapi.ORRmpolicyRatio{
			ResourceType: topoapi.ResourceType(int32(resourceType)),
			SchedulerType: topoapi.SchedulerType(int32(schedulerType)),
			RrmPolicyMemberList: &topoapi.RrmPolicyMemberList{
				RrmPolicyMember: sliceMemberList,
			},
			RrmPolicyMaxRatio:       int32(maxRatio),
			RrmPolicyMinRatio:       int32(minRatio),
			RrmPolicyDedicatedRatio: int32(dedicatedRatio),
		},
	}

	log.Infof("Seding message to sliceManager: %v", msg)
	a.sliceManager.HandleA1UpdateRequest(ctx, msg)

	a.TsPolicyTypeMap[message.PolicyId] = message.Message.Payload

	go func() {
		if message.NotificationDestination != "" {
			statusUpdateMsg := &a1tapi.PolicyStatusMessage{
				PolicyId:   message.PolicyId,
				PolicyType: message.PolicyType,
				Message: &a1tapi.StatusMessage{
					Header: &a1tapi.Header{
						RequestId:   uuid.New().String(),
						AppId:       message.Message.Header.AppId,
						Encoding:    message.Message.Header.Encoding,
						PayloadType: a1tapi.PayloadType_STATUS,
					},
				},
				NotificationDestination: message.NotificationDestination,
			}

			if message.PolicyId == SampleNotEnforcedPolicyID {
				statusUpdateMsg.Message.Payload = []byte(SampleNotEnforcedStatus)
			} else {
				statusUpdateMsg.Message.Payload = []byte(SampleEnforcedStatus)
			}

			a.StatusUpdateCh <- statusUpdateMsg
		}
	}()

	res := &a1tapi.PolicyResultMessage{
		PolicyId:   message.PolicyId,
		PolicyType: message.PolicyType,
		Message: &a1tapi.ResultMessage{
			Header: &a1tapi.Header{
				PayloadType: message.Message.Header.PayloadType,
				RequestId:   message.Message.Header.RequestId,
				Encoding:    message.Message.Header.Encoding,
				AppId:       message.Message.Header.AppId,
			}, Payload: a.TsPolicyTypeMap[message.PolicyId],
			Result: &a1tapi.Result{
				Success: true,
			},
		},
		NotificationDestination: message.NotificationDestination,
	}
	log.Info("Sending message: %v", res)
	return res, nil
}

func (a *A1PServer) PolicyDelete(ctx context.Context, message *a1tapi.PolicyRequestMessage) (*a1tapi.PolicyResultMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	log.Info("PolicyDelete called %v", message)

	var result map[string]interface{}
	json.Unmarshal(message.Message.Payload, &result)
	log.Infof("Object: %v", result)

	if message.PolicyType.Id != "ORAN_SliceSLATarget_1.0.0" {
		res := &a1tapi.PolicyResultMessage{
			PolicyId:   message.PolicyId,
			PolicyType: message.PolicyType,
			Message: &a1tapi.ResultMessage{
				Header: &a1tapi.Header{
					PayloadType: message.Message.Header.PayloadType,
					RequestId:   message.Message.Header.RequestId,
					Encoding:    message.Message.Header.Encoding,
					AppId:       message.Message.Header.AppId,
				}, Payload: message.Message.Payload,
				Result: &a1tapi.Result{
					Success: false,
					Reason:  "Policy type does not support",
				},
			},
		}
		log.Info("Sending message: %v", res)
		return res, nil
	}

	if _, ok := a.TsPolicyTypeMap[message.PolicyId]; !ok {
		res := &a1tapi.PolicyResultMessage{
			PolicyId:   message.PolicyId,
			PolicyType: message.PolicyType,
			Message: &a1tapi.ResultMessage{
				Header: &a1tapi.Header{
					PayloadType: message.Message.Header.PayloadType,
					RequestId:   message.Message.Header.RequestId,
					Encoding:    message.Message.Header.Encoding,
					AppId:       message.Message.Header.AppId,
				}, Payload: message.Message.Payload,
				Result: &a1tapi.Result{
					Success: false,
					Reason:  "Policy ID does not exists",
				},
			},
		}
		log.Info("Sending message: %v", res)
		return res, nil
	}

	delete(a.TsPolicyTypeMap, message.PolicyId)

	res := &a1tapi.PolicyResultMessage{
		PolicyId:   message.PolicyId,
		PolicyType: message.PolicyType,
		Message: &a1tapi.ResultMessage{
			Header: &a1tapi.Header{
				PayloadType: message.Message.Header.PayloadType,
				RequestId:   message.Message.Header.RequestId,
				Encoding:    message.Message.Header.Encoding,
				AppId:       message.Message.Header.AppId,
			}, Payload: a.TsPolicyTypeMap[message.PolicyId],
			Result: &a1tapi.Result{
				Success: true,
			},
		},
	}
	log.Info("Sending message: %v", res)
	return res, nil
}

func (a *A1PServer) PolicyQuery(ctx context.Context, message *a1tapi.PolicyRequestMessage) (*a1tapi.PolicyResultMessage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	log.Info("PolicyQuery called %v", message)
	var result map[string]interface{}
	json.Unmarshal(message.Message.Payload, &result)
	log.Infof("Object: %v", result)

	if message.PolicyType.Id != "ORAN_SliceSLATarget_1.0.0" {
		res := &a1tapi.PolicyResultMessage{
			PolicyId:   message.PolicyId,
			PolicyType: message.PolicyType,
			Message: &a1tapi.ResultMessage{
				Header: &a1tapi.Header{
					PayloadType: message.Message.Header.PayloadType,
					RequestId:   message.Message.Header.RequestId,
					Encoding:    message.Message.Header.Encoding,
					AppId:       message.Message.Header.AppId,
				}, Payload: message.Message.Payload,
				Result: &a1tapi.Result{
					Success: false,
					Reason:  "Policy type does not support",
				},
			},
		}
		log.Info("Sending message: %v", res)
		return res, nil
	}

	// to get all policies
	if message.PolicyId == "" {

		listPolicies := make([]string, 0)
		for k := range a.TsPolicyTypeMap {
			listPolicies = append(listPolicies, k)
		}

		listPoliciesJson, err := json.Marshal(listPolicies)
		if err != nil {
			log.Error(err)
		}

		res := &a1tapi.PolicyResultMessage{
			PolicyType: message.PolicyType,
			Message: &a1tapi.ResultMessage{
				Header: &a1tapi.Header{
					PayloadType: message.Message.Header.PayloadType,
					RequestId:   message.Message.Header.RequestId,
					Encoding:    message.Message.Header.Encoding,
					AppId:       message.Message.Header.AppId,
				}, Payload: listPoliciesJson,
				Result: &a1tapi.Result{
					Success: true,
				},
			},
		}
		log.Info("Sending message: %v", res)
		return res, nil
	}

	// checking if policy id exists or not
	if _, ok := a.TsPolicyTypeMap[message.PolicyId]; !ok {
		res := &a1tapi.PolicyResultMessage{
			PolicyId:   message.PolicyId,
			PolicyType: message.PolicyType,
			Message: &a1tapi.ResultMessage{
				Header: &a1tapi.Header{
					PayloadType: message.Message.Header.PayloadType,
					RequestId:   message.Message.Header.RequestId,
					Encoding:    message.Message.Header.Encoding,
					AppId:       message.Message.Header.AppId,
				}, Payload: message.Message.Payload,
				Result: &a1tapi.Result{
					Success: false,
					Reason:  "Policy ID does not exists",
				},
			},
		}
		log.Info("Sending message: %v", res)
		return res, nil
	}

	resultMsg := &a1tapi.PolicyResultMessage{
		PolicyId:   message.PolicyId,
		PolicyType: message.PolicyType,
		Message: &a1tapi.ResultMessage{
			Header: &a1tapi.Header{
				PayloadType: message.Message.Header.PayloadType,
				RequestId:   message.Message.Header.RequestId,
				Encoding:    message.Message.Header.Encoding,
				AppId:       message.Message.Header.AppId,
			},
			Result: &a1tapi.Result{
				Success: true,
			},
		},
	}

	switch message.Message.Header.PayloadType {
	case a1tapi.PayloadType_POLICY:
		resultMsg.Message.Payload = a.TsPolicyTypeMap[message.PolicyId]
		resultMsg.Message.Header.PayloadType = a1tapi.PayloadType_POLICY
	case a1tapi.PayloadType_STATUS:
		resultMsg.Message.Header.PayloadType = a1tapi.PayloadType_STATUS
		if message.PolicyId == SampleNotEnforcedPolicyID {
			resultMsg.Message.Payload = []byte(SampleNotEnforcedStatus)

		} else {
			resultMsg.Message.Payload = []byte(SampleEnforcedStatus)
		}
	}
	log.Info("Sending message: %v", resultMsg)
	return resultMsg, nil
}

func (a *A1PServer) PolicyStatus(server a1tapi.PolicyService_PolicyStatusServer) error {
	log.Info("PolicyStatus stream established")

	watchers := make(map[uuid.UUID]chan *a1tapi.PolicyAckMessage)
	mu := sync.RWMutex{}

	go func(m *sync.RWMutex) {
		log.Info("Start receiver for PolicyStatus")
		for {
			ack, err := server.Recv()
			if err != nil {
				log.Error(err)
			}
			m.Lock()
			log.Infof("Sending msg: %v", ack)
			for _, v := range watchers {
				select {
				case v <- ack:
					log.Infof("Sent msg %v on %v", ack, v)
				default:
					log.Infof("Failed to send msg %v on %v", ack, v)
				}
			}
			m.Unlock()
		}
	}(&a.mu)

	for msg := range a.StatusUpdateCh {
		log.Infof("Received status update message: %v", msg)
		watcherID := uuid.New()
		ackCh := make(chan *a1tapi.PolicyAckMessage)
		timerCh := make(chan bool, 1)
		go func(ch chan bool) {
			time.Sleep(5 * time.Second)
			timerCh <- true
			close(timerCh)
		}(timerCh)

		go func(m *sync.RWMutex) {
			for {
				select {
				case ack := <-ackCh:
					log.Info("ACK %v received", ack)
					if ack.Message.Header.RequestId == msg.Message.Header.RequestId {
						log.Info(ack.Message.Result.Reason)
						m.Lock()
						close(ackCh)
						delete(watchers, watcherID)
						m.Unlock()
						return
					}
				case <-timerCh:
					log.Error(fmt.Errorf("could not receive PolicyACKMessage in timer"))
					m.Lock()
					close(ackCh)
					delete(watchers, watcherID)
					m.Unlock()
					return
				}
			}
		}(&a.mu)

		mu.Lock()
		watchers[watcherID] = ackCh
		mu.Unlock()

		log.Info("Sending message: %v", msg)
		err := server.Send(msg)
		if err != nil {
			log.Error(err)
			mu.Lock()
			close(ackCh)
			delete(watchers, watcherID)
			mu.Unlock()
		}
	}

	return nil
}
