# Changelog

## [0.3.0](https://github.com/opiproject/opi-evpn-bridge/compare/v0.2.0...v0.3.0) (2024-09-02)


### Features

* complete ci and slowpath implementation of new arch ([632bc86](https://github.com/opiproject/opi-evpn-bridge/commit/632bc8641e6eb863ae2a15af47627c7812e5e4f2))
* **evpn-bridge:** netlink watcher for configuration ([46a655f](https://github.com/opiproject/opi-evpn-bridge/commit/46a655fd2ac4c208c4ec00565cce4b4f677f076e))
* **evpn:** complete fastpath implementation of new arch ([48780c6](https://github.com/opiproject/opi-evpn-bridge/commit/48780c658873abf074d52607a47f4e3b7b218cc9))


### Bug Fixes

* **evpn-bridge:** add architecture docs ([561af4b](https://github.com/opiproject/opi-evpn-bridge/commit/561af4b112bd3c05bca58b69c86442bf162e0a47))
* **evpn-bridge:** add extra netlink funcs ([2d1d2ba](https://github.com/opiproject/opi-evpn-bridge/commit/2d1d2ba1e62a01d53aa74efbd2db68e0cd3b02f0))
* **evpn-bridge:** add flow charts ([a164c52](https://github.com/opiproject/opi-evpn-bridge/commit/a164c525a71c3d86c598f3965c2b8bee1435bca1))
* **evpn-bridge:** added resourceVer check and unit test fix ([a1e051b](https://github.com/opiproject/opi-evpn-bridge/commit/a1e051bfde1b0887891c61e9eceb307c7ee5062e))
* **evpn-bridge:** delete vlan on bridge-tenant during svi teardown ([5f09e81](https://github.com/opiproject/opi-evpn-bridge/commit/5f09e810494d6e8015130624f06877940236d2aa))
* **evpn-bridge:** disable pvid on vlans for bridge ports ([5847a43](https://github.com/opiproject/opi-evpn-bridge/commit/5847a4363dfccb6c1db0ef5d608084ed7d112404))
* **evpn-bridge:** filename fix ([d33fbef](https://github.com/opiproject/opi-evpn-bridge/commit/d33fbefc0eb478f15b9063a0a7cb4f7f3ce3798a))
* **evpn-bridge:** fix empty vni use cases for vrf and logical bridge ([52e9ce7](https://github.com/opiproject/opi-evpn-bridge/commit/52e9ce71991692129de4f42204c8005f086322ae))
* **evpn-bridge:** fix getsubscribers return value check ([a1c54a7](https://github.com/opiproject/opi-evpn-bridge/commit/a1c54a7a37ca7d0d0e231ee464c0f3d8fd094cf9))
* **evpn-bridge:** fix incorrect subnet in CI ([708f948](https://github.com/opiproject/opi-evpn-bridge/commit/708f94826a3a65a81d20c2fafba3d522acf7b7d9))
* **evpn-bridge:** fix return error on no subscribers ([54ded65](https://github.com/opiproject/opi-evpn-bridge/commit/54ded65722969e7081ba3c4f257ad9a82a3e061c))
* **evpn-bridge:** go os and filename fix ([d3b20f2](https://github.com/opiproject/opi-evpn-bridge/commit/d3b20f25ede4e7dcecfc30180c08e7abd73ad72f))
* **evpn-bridge:** log file fix ([5505b13](https://github.com/opiproject/opi-evpn-bridge/commit/5505b138b946eb1f8c1dd8cfdd15a7451d98293c))
* **evpn-bridge:** make tracer optional ([a134153](https://github.com/opiproject/opi-evpn-bridge/commit/a134153ec1465b5790566c6982cdf71541b89d38))
* **evpn-bridge:** move ipu to intel e2000 ([fb49701](https://github.com/opiproject/opi-evpn-bridge/commit/fb4970122d1eacf845b3c19b210ec1afafb41e76))
* **evpn-bridge:** moving from uint16 to int16 ([6b422c4](https://github.com/opiproject/opi-evpn-bridge/commit/6b422c45c8ac15ecbab0e5d1d1df449b9f8bacf4))
* **evpn-bridge:** netlink neighbor fix ([aeedab7](https://github.com/opiproject/opi-evpn-bridge/commit/aeedab7bb563aa4f8e4381a5359f3c64f5713df0))
* **evpn-bridge:** retry when the routing table is busy ([7c0854f](https://github.com/opiproject/opi-evpn-bridge/commit/7c0854f0e8d6cf9af479a132d05532a596e148cd))
* **evpn-bridge:** revisions after review ([0ad2a00](https://github.com/opiproject/opi-evpn-bridge/commit/0ad2a003558cdb49c7c367bf89ba146255737f30))
* **evpn-bridge:** revisions after review ([522b415](https://github.com/opiproject/opi-evpn-bridge/commit/522b41568516ca72a18caf77b2814fb3412923da))
* **evpn-bridge:** validate config and frr addr fix ([30c54c9](https://github.com/opiproject/opi-evpn-bridge/commit/30c54c9c8a72be2d35e7c277cfb79db5bf6d09ef))
* **evpn:** fix graceful shutdown, make tracer optional ([4829ecd](https://github.com/opiproject/opi-evpn-bridge/commit/4829ecd7aeab715ca4cd951ec5896477e8b6c836))
* **evpn:** minor syntax improvements in netlink ([14dcaaa](https://github.com/opiproject/opi-evpn-bridge/commit/14dcaaa642905cca87493ec60c683a63b60c8fa6))
* **evpn:** netlink fixes ([d7b6081](https://github.com/opiproject/opi-evpn-bridge/commit/d7b6081e78af7173dfdc8d8adac9d1f96fb9c86c))
* **evpn:** netlink PR comments ([eaad649](https://github.com/opiproject/opi-evpn-bridge/commit/eaad6490e8f21b9b103bbd39edae92641990e8fc))
* **evpn:** netlink reviews ([cbc1fba](https://github.com/opiproject/opi-evpn-bridge/commit/cbc1fba443fee73d6c8351d5dce1d72291c29bef))
* **evpn:** netlink split changes ([0596327](https://github.com/opiproject/opi-evpn-bridge/commit/0596327a7679b4f706abdcb72ecbd6514522a646))
* **evpn:** netlink split changes ([e742efa](https://github.com/opiproject/opi-evpn-bridge/commit/e742efa7cb47a612f0ee1b49d72b1c8d44f211e2))
* **evpn:** netlink split changes ([cced04b](https://github.com/opiproject/opi-evpn-bridge/commit/cced04b2812d51ddb35ce6e4e7d9819ab38dc2ad))
* **evpn:** remove vendor-plugins and related config ([8f51de1](https://github.com/opiproject/opi-evpn-bridge/commit/8f51de196b54c22c437741d8aa20599e5d71b80e))
* **evpn:** split netlink files ([58f0046](https://github.com/opiproject/opi-evpn-bridge/commit/58f0046595a4c125244acb7845d5a7b2c7ee498a))
* **evpn:** update netlink split changes ([971033e](https://github.com/opiproject/opi-evpn-bridge/commit/971033efa223e87845d97c16716238633eb437fc))
* **netlink:** complete working netlink ([7b98801](https://github.com/opiproject/opi-evpn-bridge/commit/7b98801f6bf02247d3b5bda54571e83154ab8162))
