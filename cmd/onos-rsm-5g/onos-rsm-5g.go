// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"

	"github.com/onosproject/onos-lib-go/pkg/certs"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-rsm-5g/pkg/manager"
)

var log = logging.GetLogger()

func main() {
	caPath := flag.String("caPath", "", "path to CA certificate")
	keyPath := flag.String("keyPath", "", "path to client private key")
	certPath := flag.String("certPath", "", "path to client certificate")
	configPath := flag.String("configPath", "/etc/onos/config/config.json", "path to config.json file")
	e2tEndpoint := flag.String("e2tEndpoint", "onos-e2t:5150", "E2T service endpoint")
	grpcPort := flag.Int("grpcPort", 5150, "grpc Port number")
	smName := flag.String("smName", "oran-e2sm-ccc", "Service model name in RAN function description")
	smVersion := flag.String("smVersion", "v1", "Service model version in RAN function description")
	appID := flag.String("appID", "onos-rsm-5g", "ONOS-RSM-5G xApp ID")
	ackTimer := flag.Int("ackTimer", 5, "ACK timer (seconds)")

	ready := make(chan bool)

	flag.Parse()

	_, err := certs.HandleCertPaths(*caPath, *keyPath, *certPath, true)
	if err != nil {
		log.Fatal(err)
	}

	log.Info("Starting onos-rsm-5g")

	cfg := manager.Config{
		CAPath:      *caPath,
		KeyPath:     *keyPath,
		CertPath:    *certPath,
		ConfigPath:  *configPath,
		E2tEndpoint: *e2tEndpoint,
		GRPCPort:    *grpcPort,
		SMName:      *smName,
		SMVersion:   *smVersion,
		AppID:       *appID,
		AckTimer:    *ackTimer,
	}

	mgr := manager.NewManager(cfg)
	mgr.Run()
	<-ready
}
