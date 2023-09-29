# SPDX-FileCopyrightText: 2023-present Intel Corporation
#
# SPDX-License-Identifier: Apache-2.0

ARG ONOS_BUILD_VERSION=undefined

FROM onosproject/golang-build:v1.3.0 as build

ENV GO111MODULE=on
ARG ONOS_MAKE_TARGET=build

COPY Makefile go.mod go.sum /go/src/github.com/onosproject/onos-rsm-5g/
COPY cmd/ /go/src/github.com/onosproject/onos-rsm-5g/cmd/
COPY pkg/ /go/src/github.com/onosproject/onos-rsm-5g/pkg/
COPY vendor/ /go/src/github.com/onosproject/onos-rsm-5g/vendor/
COPY api/ /go/src/github.com/onosproject/onos-rsm-5g/api
COPY build/build-tools/ /go/src/github.com/onosproject/onos-rsm-5g/build/build-tools/

RUN cd /go/src/github.com/onosproject/onos-rsm-5g && GOFLAGS=-mod=vendor make ${ONOS_MAKE_TARGET}

FROM frolvlad/alpine-glibc:alpine-3.12

USER nobody

COPY --from=build /go/src/github.com/onosproject/onos-rsm-5g/build/_output/onos-rsm-5g /usr/local/bin/onos-rsm-5g

ENTRYPOINT ["onos-rsm-5g"]
