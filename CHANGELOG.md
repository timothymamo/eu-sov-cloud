# Changelog

## [0.3.0](https://github.com/timothymamo/shift2eu/compare/v0.2.0...v0.3.0) (2026-11-12)


### Features

* **dev:** set the deck's default_command for new agents in make vm-init ([#135](https://github.com/timothymamo/shift2eu/issues/135)) ([60b0e07](https://github.com/timothymamo/shift2eu/commit/60b0e0757566d743b8d0e23dc90e2eb874f12ff1)), closes [#134](https://github.com/timothymamo/shift2eu/issues/134)
* **dev:** set up a recreated VM with make vm-init and vm-create ([#131](https://github.com/timothymamo/shift2eu/issues/131)) ([0cff76f](https://github.com/timothymamo/shift2eu/commit/0cff76f47d048da7f8dafad5a4491bf756c7f9ab))
* **release:** ship shift2eu-server in its own release archives ([#162](https://github.com/timothymamo/shift2eu/issues/162)) ([1189e9d](https://github.com/timothymamo/shift2eu/commit/1189e9d7d27b1d34abc35447b5701f3038d8dc1a))
* **release:** ship shift2eu-server with GoReleaser and as a signed image (PRD [#103](https://github.com/timothymamo/shift2eu/issues/103) M4) ([#140](https://github.com/timothymamo/shift2eu/issues/140)) ([0c5e067](https://github.com/timothymamo/shift2eu/commit/0c5e067361c304a05441e3a5ca3f2157375a95cd))


### Bug Fixes

* **dev:** drop allow rules whose wildcard lets git and make options through ([#133](https://github.com/timothymamo/shift2eu/issues/133)) ([ae70409](https://github.com/timothymamo/shift2eu/commit/ae70409dd18134f412b01f32e22cc79b954c8b9a)), closes [#132](https://github.com/timothymamo/shift2eu/issues/132)
* **lint:** forbid process execution and environment reads in the composition roots ([#111](https://github.com/timothymamo/shift2eu/issues/111)) ([#150](https://github.com/timothymamo/shift2eu/issues/150)) ([064b9da](https://github.com/timothymamo/shift2eu/commit/064b9da598e2a621c7cfd1c14df038ef4e5336eb))
* **lint:** make adapters-are-built-in-cmd fire and guard every depguard rule against shadowing ([#119](https://github.com/timothymamo/shift2eu/issues/119)) ([#143](https://github.com/timothymamo/shift2eu/issues/143)) ([f13e4e3](https://github.com/timothymamo/shift2eu/commit/f13e4e32791a9f559225789d845e1b6bde572809))
* **lint:** make app-tests-use-fakes-not-adapters a strict allow list ([#144](https://github.com/timothymamo/shift2eu/issues/144)) ([#154](https://github.com/timothymamo/shift2eu/issues/154)) ([da346a8](https://github.com/timothymamo/shift2eu/commit/da346a8054f98305524bb868b60c32d1714fcae0))
