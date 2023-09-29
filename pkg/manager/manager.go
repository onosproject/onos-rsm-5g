// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"context"
	"strconv"
	"strings"

	"github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-lib-go/pkg/northbound"
	app "github.com/onosproject/onos-ric-sdk-go/pkg/config/app/default"
	"github.com/onosproject/onos-rsm-5g/pkg/broker"
	appConfig "github.com/onosproject/onos-rsm-5g/pkg/config"
	"github.com/onosproject/onos-rsm-5g/pkg/rnib"
	nbi "github.com/onosproject/onos-rsm-5g/pkg/northbound"
	"github.com/onosproject/onos-rsm-5g/pkg/northbound/a1"
	"github.com/onosproject/onos-rsm-5g/pkg/slicing"
	"github.com/onosproject/onos-rsm-5g/pkg/southbound/e2"
)

var log = logging.GetLogger()

// Config is a manager configuration
type Config struct {
	CAPath      string
	KeyPath     string
	CertPath    string
	ConfigPath  string
	E2tEndpoint string
	GRPCPort    int
	AppConfig   *app.Config
	SMName      string
	SMVersion   string
	AppID       string
	AckTimer    int
}

func NewManager(config Config) *Manager {
	appCfg, err := appConfig.NewConfig(config.ConfigPath)
	if err != nil {
		log.Warn(err)
	}
	subscriptionBroker := broker.NewBroker()
	rnibClient, err := rnib.NewClient()
	if err != nil {
		log.Warn(err)
	}

	ctrlReqChsSliceUpdate := make(map[string]chan *e2.CtrlMsg)

	cccReqCh := make(chan *nbi.CccMsg)

	slicingManager := slicing.NewManager(
		slicing.WithRnibClient(rnibClient),
		slicing.WithCtrlReqChs(ctrlReqChsSliceUpdate),
		slicing.WithNbiReqChs(cccReqCh),
		slicing.WithAckTimer(config.AckTimer),
	)

	e2tHostAddr := strings.Split(config.E2tEndpoint, ":")[0]
	e2tPort, err := strconv.Atoi(strings.Split(config.E2tEndpoint, ":")[1])
	if err != nil {
		log.Warn(err)
	}

	e2Manager, err := e2.NewManager(
		e2.WithE2TAddress(e2tHostAddr, e2tPort),
		e2.WithServiceModel(e2.ServiceModelName(config.SMName), e2.ServiceModelVersion(config.SMVersion)),
		e2.WithAppConfig(appCfg),
		e2.WithAppID(config.AppID),
		e2.WithBroker(subscriptionBroker),
		e2.WithRnibClient(rnibClient),
		e2.WithCtrlReqChs(ctrlReqChsSliceUpdate),
	)
	if err != nil {
		log.Warn(err)
	}

	a1PolicyTypes := make([]*topo.A1PolicyType, 0)
	a1Policy := &topo.A1PolicyType{
		Name: "ORAN_SliceSLATarget",
		Version: "1.0.0",
		ID: "ORAN_SliceSLATarget_1.0.0",
		Description: "O-RAN standard slice SLA policy",
	}
	a1PolicyTypes = append(a1PolicyTypes, a1Policy)

	a1Manager, err := a1.NewManager(config.CAPath, config.KeyPath, config.CertPath, config.GRPCPort, config.AppID, a1PolicyTypes, slicingManager)
	if err != nil {
		log.Warn(err)
	}

	return &Manager{
		appConfig:             appCfg,
		config:                config,
		e2Manager:             e2Manager,
		a1Manager:             *a1Manager,
		rnibClient:            rnibClient,
		slicingManager:        slicingManager,
		ctrlReqChsSliceUpdate: ctrlReqChsSliceUpdate,
		cccReqCh:              cccReqCh,
	}
}

// Manager is a manager for the CCC xAPP service
type Manager struct {
	appConfig             appConfig.Config
	config                Config
	e2Manager             e2.Manager
	a1Manager             a1.Manager
	rnibClient            rnib.Client
	slicingManager        slicing.Manager
	ctrlReqChsSliceUpdate map[string]chan *e2.CtrlMsg
	cccReqCh              chan *nbi.CccMsg
}

// Run starts the manager and the associated services
func (m *Manager) Run() {
	log.Info("Running Manager")
	if err := m.start(); err != nil {
		log.Fatal("Unable to run Manager: %v", err)
	}
}

func (m *Manager) start() error {
	err := m.startNorthboundServer()
	if err != nil {
		log.Warn(err)
		return err
	}

	err = m.e2Manager.Start()
	if err != nil {
		log.Warn(err)
		return err
	}

	m.a1Manager.Start()

	go m.slicingManager.Run(context.Background())

	return nil
}

func (m *Manager) Close() {
	log.Info("Closing Manager")
	m.a1Manager.Close(context.Background())
}

func (m *Manager) startNorthboundServer() error {
	s := northbound.NewServer(northbound.NewServerCfg(
		m.config.CAPath,
		m.config.KeyPath,
		m.config.CertPath,
		int16(m.config.GRPCPort),
		true,
		northbound.SecurityConfig{}))

	s.AddService(nbi.NewService(m.rnibClient, m.cccReqCh))
	s.AddService(a1.NewA1EIService())
	s.AddService(a1.NewA1PService(m.slicingManager))

	doneCh := make(chan error)
	go func() {
		err := s.Serve(func(started string) {
			log.Info("Started NBI on ", started)
			close(doneCh)
		})
		if err != nil {
			doneCh <- err
		}
	}()
	return <-doneCh
}
