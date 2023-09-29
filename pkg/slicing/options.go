// SPDX-FileCopyrightText: 2020-present Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package slicing

import (
	"github.com/onosproject/onos-rsm-5g/pkg/rnib"
	"github.com/onosproject/onos-rsm-5g/pkg/northbound"
	"github.com/onosproject/onos-rsm-5g/pkg/southbound/e2"
)

type Options struct {
	Chans Channels

	App AppOptions
}

type Channels struct {
	CccMsgCh chan *northbound.CccMsg
	CtrlReqChsSliceUpdate map[string]chan *e2.CtrlMsg
}

type AppOptions struct {
	RnibClient rnib.Client
	AckTimer int
}

type Option interface {
	apply(*Options)
}

type funcOption struct {
	f func(*Options)
}

func (f funcOption) apply(options *Options) {
	f.f(options)
}

func newOption(f func(*Options)) Option {
	return funcOption{
		f: f,
	}
}

func WithCtrlReqChs(ctrlReqChsSliceUpdate map[string]chan *e2.CtrlMsg) Option {
	return newOption(func(options *Options) {
		options.Chans.CtrlReqChsSliceUpdate = ctrlReqChsSliceUpdate
	})
}

func WithNbiReqChs(cccMsgCh chan *northbound.CccMsg) Option {
	return newOption(func(options *Options) {
		options.Chans.CccMsgCh = cccMsgCh
	})
}

func WithRnibClient(rnibClient rnib.Client) Option {
	return newOption(func(options *Options) {
		options.App.RnibClient = rnibClient
	})
}

func WithAckTimer(ackTimer int) Option {
	return newOption(func(options *Options) {
		options.App.AckTimer = ackTimer
	})
}
