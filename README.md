# OPI gRPC to EVPN GW FRR bridge

[![Linters](https://github.com/opiproject/opi-evpn-bridge/actions/workflows/linters.yml/badge.svg)](https://github.com/opiproject/opi-evpn-bridge/actions/workflows/linters.yml)
[![CodeQL](https://github.com/opiproject/opi-evpn-bridge/actions/workflows/codeql.yml/badge.svg)](https://github.com/opiproject/opi-evpn-bridge/actions/workflows/codeql.yml)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/opiproject/opi-evpn-bridge/badge)](https://securityscorecards.dev/viewer/?platform=github.com&org=opiproject&repo=opi-evpn-bridge)
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

This repo includes OPI reference code for EVPN Gateway based on [FRR](https://www.frrouting.org/). The specification for these APIs can be found
[here](https://github.com/opiproject/opi-api/pull/276).

## I Want To Contribute

This project welcomes contributions and suggestions.  We are happy to have the Community involved via submission of **Issues and Pull Requests** (with substantive content or even just fixes). We are hoping for the documents, test framework, etc. to become a community process with active engagement.  PRs can be reviewed by by any number of people, and a maintainer may accept.

See [CONTRIBUTING](https://github.com/opiproject/opi/blob/main/CONTRIBUTING.md) and [GitHub Basic Process](https://github.com/opiproject/opi/blob/main/doc-github-rules.md) for more details.

## Getting started

Run `docker-compose up -d`

## Manual gRPC example

```bash
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"vrf" : {"spec" : {"vni" : 1234, "loopback_ip_prefix" : {"addr": {"af": "IP_AF_INET", "v4_addr": 167772162} }, "len": 24}, "vtep_ip_prefix": {"addr": {"af": "IP_AF_INET", "v4_addr": 167772162} }, "len": 24} }}, "vrf_id" : "testvrf" }' localhost:50151 opi_api.network.evpn-gw.v1alpha1.VrfService.CreateVrf"
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"logical_bridge" : {"spec" : {"vni": 10, "vlan_id": 10 } }, "logical_bridge_id" : "testbridge" }' localhost:50151 opi_api.network.evpn-gw.v1alpha1.LogicalBridgeService.CreateLogicalBridge
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"bridge_port" : {"spec" : {mac_address: "qrvMAAAB", "ptype": "ACCESS", "logical_bridges": ["//network.opiproject.org/bridges/testbridge"] }}, "bridge_port_id" : "testport"}' localhost:50151 opi_api.network.evpn-gw.v1alpha1.BridgePortService.CreateBridgePort
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"tunnel" : {"spec" : {"vpc_name_ref": "//network.opiproject.org/bridges/testbridge", "local_ip": {"af": "IP_AF_INET", "v4_addr": 336860161}, "encap": {"type": "ENCAP_TYPE_VXLAN", "value": {"vnid": 100}} } }, "tunnel_id" : "testvxlan", "parent" : "todo" }' localhost:50151 opi_api.network.cloud.v1alpha1.CloudInfraService.CreateTunnel
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"name": "//network.opiproject.org/ports/testinterface"}' localhost:50151 opi_api.network.evpn-gw.v1alpha1.BridgePortService.GetBridgePort
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"name": "//network.opiproject.org/bridges/testbridge"}' localhost:50151 opi_api.network.evpn-gw.v1alpha1.LogicalBridgeService.GetLogicalBridge
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"name": "//network.opiproject.org/tunnels/testvxlan"}' localhost:50151 opi_api.network.cloud.v1alpha1.CloudInfraService.GetTunnel
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"name": "//network.opiproject.org/vrfs/testvrf"}' localhost:50151 opi_api.network.evpn-gw.v1alpha1.VrfService.GetVrf
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"name": "//network.opiproject.org/ports/testinterface"}' localhost:50151 opi_api.network.evpn-gw.v1alpha1.BridgePortService.DeleteBridgePort
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"name": "//network.opiproject.org/bridges/testbridge"}' localhost:50151 opi_api.network.evpn-gw.v1alpha1.LogicalBridgeService.DeleteLogicalBridge
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"name" : "//network.opiproject.org/tunnels/testvxlan"}' localhost:50151 opi_api.network.cloud.v1alpha1.CloudInfraService.DeleteTunnel
docker-compose exec opi-evpn-bridge grpcurl -plaintext -d '{"name" : "//network.opiproject.org/vrfs/testvrf"}' localhost:50151 opi_api.network.evpn-gw.v1alpha1.VrfService.DeleteVrf
```

## Architecture Diagram

![OPI EVPN Bridge Architcture Diagram](./docs/OPI-EVPN-GW-FRR-bridge.png)

## POC diagrams

![OPI EVPN Bridge POC Diagram for CI/CD](./docs/OPI-EVPN-PoC.png)
![OPI EVPN Bridge Diagram for L2VXLAN](./docs/OPI-EVPN-L2-VXLAN.png)
![OPI EVPN Bridge Diagram for L3VXLAN Asymmetric IRB](./docs/OPI-EVPN-L3-Asymmetric-IRB.png)
![OPI EVPN Bridge Diagram for L3VXLAN Symmetric IRB](./docs/OPI-EVPN-L3-Symmetric-IRB.png)
![OPI EVPN Bridge Diagram for L2VXLAN in_Symmetric IRB](./docs/OPI-EVPN-L2-VXLAN-In-Symmetric-IRB-setup.png)
![OPI EVPN Bridge Diagram for Leaf1_Detailed_View](./docs/OPI-EVPN-Leaf1-Detailed-View.png)
