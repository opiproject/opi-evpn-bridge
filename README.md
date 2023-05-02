# OPI gRPC to EVPN GW FRR bridge

[![Linters](https://github.com/opiproject/opi-evpn-bridge/actions/workflows/linters.yml/badge.svg)](https://github.com/opiproject/opi-evpn-bridge/actions/workflows/linters.yml)
[![tests](https://github.com/opiproject/opi-evpn-bridge/actions/workflows/go.yml/badge.svg)](https://github.com/opiproject/opi-evpn-bridge/actions/workflows/go.yml)
[![Docker](https://github.com/opiproject/opi-evpn-bridge/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/opiproject/opi-evpn-bridge/actions/workflows/docker-publish.yml)
[![License](https://img.shields.io/github/license/opiproject/opi-evpn-bridge?style=flat-square&color=blue&label=License)](https://github.com/opiproject/opi-evpn-bridge/blob/master/LICENSE)
[![codecov](https://codecov.io/gh/opiproject/opi-evpn-bridge/branch/main/graph/badge.svg)](https://codecov.io/gh/opiproject/opi-evpn-bridge)
[![Go Report Card](https://goreportcard.com/badge/github.com/opiproject/opi-evpn-bridge)](https://goreportcard.com/report/github.com/opiproject/opi-evpn-bridge)
[![Go Doc](https://img.shields.io/badge/godoc-reference-blue.svg)](http://godoc.org/github.com/opiproject/opi-evpn-bridge)
[![Pulls](https://img.shields.io/docker/pulls/opiproject/opi-evpn-bridge.svg?logo=docker&style=flat&label=Pulls)](https://hub.docker.com/r/opiproject/opi-evpn-bridge)
[![Last Release](https://img.shields.io/github/v/release/opiproject/opi-evpn-bridge?label=Latest&style=flat-square&logo=go)](https://github.com/opiproject/opi-evpn-bridge/releases)
[![GitHub stars](https://img.shields.io/github/stars/opiproject/opi-evpn-bridge.svg?style=flat-square&label=github%20stars)](https://github.com/opiproject/opi-evpn-bridge)
[![GitHub Contributors](https://img.shields.io/github/contributors/opiproject/opi-evpn-bridge.svg?style=flat-square)](https://github.com/opiproject/opi-evpn-bridge/graphs/contributors)

This directory contains the security PoC for OPI. This includes reference code
for the [IPsec](https://github.com/opiproject/opi-api/blob/main/security/v1/ipsec.proto)
APIs. The specification for these APIs can be found
[here](https://github.com/opiproject/opi-api/blob/main/security/v1/autogen.md).

## I Want To Contribute

This project welcomes contributions and suggestions.  We are happy to have the Community involved via submission of **Issues and Pull Requests** (with substantive content or even just fixes). We are hoping for the documents, test framework, etc. to become a community process with active engagement.  PRs can be reviewed by by any number of people, and a maintainer may accept.

See [CONTRIBUTING](https://github.com/opiproject/opi/blob/main/CONTRIBUTING.md) and [GitHub Basic Process](https://github.com/opiproject/opi/blob/main/doc-github-rules.md) for more details.

## Getting started

Run `docker-compose up -d`

## Architecture Diagram

![OPI EVPN Bridge Architcture Diagram](./OPI-EVPN-GW-FRR-bridge.png)

## POC diagram

![OPI EVPN Bridge POC Diagram for CI/CD](./OPI-EVPN-PoC.png)
