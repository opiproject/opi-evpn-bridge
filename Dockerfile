# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2022-2023 Dell Inc, or its subsidiaries.

FROM docker.io/library/golang:1.21.6-alpine as builder

WORKDIR /app
# Download necessary Go modules
COPY config.yaml ./
COPY go.mod ./
COPY go.sum ./
RUN go mod download
# build an app
COPY cmd/ cmd/
COPY pkg/ pkg/
RUN go build -v -o /opi-evpn-bridge /app/cmd/...

# second stage to reduce image size
FROM alpine:3.19
RUN apk add --no-cache --no-check-certificate hwdata && rm -rf /var/cache/apk/*
COPY --from=builder /opi-evpn-bridge /
COPY --from=docker.io/fullstorydev/grpcurl:v1.8.9-alpine /bin/grpcurl /usr/local/bin/
COPY --from=builder /app/config.yaml /
RUN apk add --no-cache iproute2 && \
	mkdir -p /etc/iproute2/ && \
	echo "255     opi_evpn_br" > /etc/iproute2/rt_protos && \
	cat /etc/iproute2/rt_protos && \
	ls -al /
EXPOSE 50051 8082
CMD [ "/opi-evpn-bridge", "--grpcport=50051", "--httpport=8082"]
HEALTHCHECK CMD grpcurl -plaintext localhost:50051 list || exit 1
