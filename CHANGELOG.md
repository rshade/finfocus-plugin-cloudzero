# Changelog

## [0.1.3](https://github.com/rshade/finfocus-plugin-cloudzero/compare/v0.1.2...v0.1.3) (2026-11-20)


### Features

* hold the rate-limit cooldown for the wait CloudZero states ([a5ad2bd](https://github.com/rshade/finfocus-plugin-cloudzero/commit/a5ad2bde5df88f14cf564f0da5930f5501c8c0fc)), closes [#26](https://github.com/rshade/finfocus-plugin-cloudzero/issues/26)

## [0.1.2](https://github.com/rshade/finfocus-plugin-cloudzero/compare/v0.1.1...v0.1.2) (2026-11-20)


### Features

* serve CloudZero monthly budgets through GetBudgets ([55c7476](https://github.com/rshade/finfocus-plugin-cloudzero/commit/55c74764d8ccf4d984f4451c0d2ead40447d1305))
* serve CloudZero recommendations, including root-volume ones on their instance ([930497b](https://github.com/rshade/finfocus-plugin-cloudzero/commit/930497b7797266a8e7c5dab087058987aab0cb13)), closes [#29](https://github.com/rshade/finfocus-plugin-cloudzero/issues/29)


### Documentation

* promote [#9](https://github.com/rshade/finfocus-plugin-cloudzero/issues/9) and track the cooldown follow-up ([4909ce9](https://github.com/rshade/finfocus-plugin-cloudzero/commit/4909ce93737c56bd73709136de5ad1e0d278afec))
* sync the roadmap after v0.1.1 ([21d27f8](https://github.com/rshade/finfocus-plugin-cloudzero/commit/21d27f887dad59a99631a21e884779baf5075ea3))

## [0.1.1](https://github.com/rshade/finfocus-plugin-cloudzero/compare/v0.1.0...v0.1.1) (2026-11-19)


### Features

* include an instance's root EBS volume in its actual cost ([dddb4c7](https://github.com/rshade/finfocus-plugin-cloudzero/commit/dddb4c74d4e43747e0e3857469f55b8ee40feab5)), closes [#10](https://github.com/rshade/finfocus-plugin-cloudzero/issues/10)
* report CloudZero's rate-limit wait in the ResourceExhausted message ([38a982c](https://github.com/rshade/finfocus-plugin-cloudzero/commit/38a982cbf50bdeddf0b7cc56de0d3981cf74c08b)), closes [#11](https://github.com/rshade/finfocus-plugin-cloudzero/issues/11)
* report CloudZero's rate-limit wait in the ResourceExhausted message ([#23](https://github.com/rshade/finfocus-plugin-cloudzero/issues/23)) ([3adfebf](https://github.com/rshade/finfocus-plugin-cloudzero/commit/3adfebf3b1d694ff9b2c56796eaa8fbf3c399249))


### Bug Fixes

* **deps:** update module google.golang.org/grpc to v1.85.0 ([5140a74](https://github.com/rshade/finfocus-plugin-cloudzero/commit/5140a74efc36db88c7777749b65facaa67788768))
* **deps:** update module google.golang.org/grpc to v1.85.0 ([95864f8](https://github.com/rshade/finfocus-plugin-cloudzero/commit/95864f8ac71ffb0cac3713bbd7726714838577ed))
* describe the CloudZero rate limit as a rolling 24 hours ([835122f](https://github.com/rshade/finfocus-plugin-cloudzero/commit/835122fa360ce0a373edbc017626d9ae3bccfa7b))
* report cloudzero as the plugin name ([8319e3b](https://github.com/rshade/finfocus-plugin-cloudzero/commit/8319e3b466f0b8ba3a6213c87eedcedb3047ae37)), closes [#17](https://github.com/rshade/finfocus-plugin-cloudzero/issues/17)
* share concurrent CloudZero cost queries and cool down after a 429 ([#20](https://github.com/rshade/finfocus-plugin-cloudzero/issues/20)) ([0980404](https://github.com/rshade/finfocus-plugin-cloudzero/commit/098040435d213b8a1a55b44af0bea03ca3679522)), closes [#19](https://github.com/rshade/finfocus-plugin-cloudzero/issues/19)


### Documentation

* add CONTEXT.md and ROADMAP.md and retire TASKS.md ([4f8605c](https://github.com/rshade/finfocus-plugin-cloudzero/commit/4f8605c690f3e804768c98332fbc0d4fbae7ad31))
* add CONTEXT.md, ROADMAP.md, and baseline OpenSpec specs ([b55c8d0](https://github.com/rshade/finfocus-plugin-cloudzero/commit/b55c8d019daad8681ac853b886147f434c63447c))
* format the registry PR reference in TASKS.md ([b99f154](https://github.com/rshade/finfocus-plugin-cloudzero/commit/b99f1540a69e5c4b3b4dd912e5940a12b87483b1))
* link the roadmap items to their issues ([e0b2d4d](https://github.com/rshade/finfocus-plugin-cloudzero/commit/e0b2d4d8e0267955205cb58f6e618e4897fb5254))
* link the roadmap items to their issues ([f5bec1f](https://github.com/rshade/finfocus-plugin-cloudzero/commit/f5bec1f64ffa0e4ad1847df430285c6c5b1f5578))
* record the end_date and --adapter spike results ([867eb08](https://github.com/rshade/finfocus-plugin-cloudzero/commit/867eb08f65ac795d1a9b879608e9f003e0cd4517))
* record the end_date and --adapter spike results ([78eaaaf](https://github.com/rshade/finfocus-plugin-cloudzero/commit/78eaaafca74106c9d88afffeba4045f9ecb2cb88))
* record the v0.1.0 release and live check in TASKS.md ([faf6560](https://github.com/rshade/finfocus-plugin-cloudzero/commit/faf6560ee85811e29f608bedbeb40ed8e0df0fc7))

## 0.1.0 (2026-11-18)


### Features

* add CloudZero actual-cost plugin v0.1.0 ([a2b6660](https://github.com/rshade/finfocus-plugin-cloudzero/commit/a2b6660c001941004ba07e122c5297feb725407c))


### Bug Fixes

* match AWS resources without an ARN by CZRN suffix ([72691d7](https://github.com/rshade/finfocus-plugin-cloudzero/commit/72691d7128a5f29c8a3a6ad20756c23cd8b061ef))
* match CloudProvider from the resource's provider instead of a fixed table ([b695d05](https://github.com/rshade/finfocus-plugin-cloudzero/commit/b695d05bc240bfc91ce3bd647dfbc76537e5951f))
