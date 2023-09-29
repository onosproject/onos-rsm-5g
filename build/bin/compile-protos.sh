#!/bin/sh

# SPDX-FileCopyrightText: 2023-present Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0

proto_imports=".:${GOPATH}/src/github.com/gogo/protobuf/protobuf:${GOPATH}/src/github.com/gogo/protobuf:${GOPATH}/src/github.com/envoyproxy/protoc-gen-validate:${GOPATH}/src":"${GOPATH}/src/github.com/onosproject/onos-rsm-5g/api"

cp -r github.com/onosproject/onos-rsm-5g/* .
rm -rf github.com
