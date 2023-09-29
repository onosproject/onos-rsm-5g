// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package northbound

import (
	"context"
	cccapi "github.com/onosproject/onos-api/go/onos/ccc"
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/logging/service"
	"github.com/onosproject/onos-rsm-5g/pkg/rnib"
	"google.golang.org/grpc"
)

func NewService(rnibClient rnib.Client, cccReqCh chan *CccMsg) service.Service {
	return &Service{
		rnibClient:  rnibClient,
		cccReqCh:    cccReqCh,
	}
}

type Service struct {
	rnibClient  rnib.Client
	cccReqCh    chan *CccMsg
}

func (s Service) Register(r *grpc.Server) {
	server := &Server{
		rnibClient:  s.rnibClient,
		cccReqCh:    s.cccReqCh,
	}
	cccapi.RegisterCccServer(r, server)
}

type Server struct {
	rnibClient  rnib.Client
	cccReqCh    chan *CccMsg
}

func (s Server) UpdateSlice(ctx context.Context, request *cccapi.UpdateSliceRequest) (*cccapi.UpdateSliceResponse, error) {
	ackCh := make(chan Ack)
	msg := &CccMsg{
		NodeID:  topoapi.ID(request.E2NodeId),
		Message: request,
		AckCh:   ackCh,
	}
	go func(msg *CccMsg) {
		s.cccReqCh <- msg
	}(msg)

	ack := <-ackCh
	return &cccapi.UpdateSliceResponse{
		Ack: &cccapi.Ack{
			Success: ack.Success,
			Cause:   ack.Reason,
		},
	}, nil
}
