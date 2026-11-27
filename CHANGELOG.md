# Changelog

## [0.12.0](https://github.com/raqeebshafeeque/flowd/compare/v0.11.0...v0.12.0) (2026-11-27)


### Features

* order the toolbar, ask y or n for reset and save forms with enter ([#57](https://github.com/raqeebshafeeque/flowd/issues/57)) ([7c78aba](https://github.com/raqeebshafeeque/flowd/commit/7c78abad102a420b99b25d652cae568b487533aa))

## [0.11.0](https://github.com/raqeebshafeeque/flowd/compare/v0.10.0...v0.11.0) (2026-11-20)


### ⚠ BREAKING CHANGES

* the skipped status and the skip, unskip, resume, reopen and events commands are removed. Use start to resume or reopen an item. --timezone and now --max are removed, and a bare date like flowd 2026-11-18 is no longer a command. The JSON schema version is 5, and status_counts no longer has skipped. Databases with skipped items are not supported.

### Code Refactoring

* make flowd lean, and add reset ([#55](https://github.com/raqeebshafeeque/flowd/issues/55)) ([dc0eb9d](https://github.com/raqeebshafeeque/flowd/commit/dc0eb9da37fd2356ba42f0657afd0a8ca3d50910))

## [0.10.0](https://github.com/raqeebshafeeque/flowd/compare/v0.9.0...v0.10.0) (2026-11-20)


### Features

* put a forgotten finish right, and write down the principles ([#53](https://github.com/raqeebshafeeque/flowd/issues/53)) ([233d754](https://github.com/raqeebshafeeque/flowd/commit/233d754a601ebdd0a95ce00060e4137b90f0b4ba))

## [0.9.0](https://github.com/raqeebshafeeque/flowd/compare/v0.8.0...v0.9.0) (2026-11-19)


### Features

* carry incomplete and skipped items forward, by choice ([#50](https://github.com/raqeebshafeeque/flowd/issues/50)) ([9cc6ed2](https://github.com/raqeebshafeeque/flowd/commit/9cc6ed2dc3544b44b2b98b9b24c7aff28cb2ca0b))
* **cli:** add tab completion, an item picker and flowd skill ([#47](https://github.com/raqeebshafeeque/flowd/issues/47)) ([3631c68](https://github.com/raqeebshafeeque/flowd/commit/3631c6871597fa2c95ca8a2be327a33f03fca5d8))
* put the day in your own order, saved with Ctrl+S ([#51](https://github.com/raqeebshafeeque/flowd/issues/51)) ([c184e5f](https://github.com/raqeebshafeeque/flowd/commit/c184e5fb58a4fb5faca534edb91620437b6b65d1))

## [0.8.0](https://github.com/raqeebshafeeque/flowd/compare/v0.7.1...v0.8.0) (2026-11-19)


### Features

* **cli:** add flowd now --short for status bars and prompts ([#45](https://github.com/raqeebshafeeque/flowd/issues/45)) ([828787a](https://github.com/raqeebshafeeque/flowd/commit/828787a07d128dc07ca79779b7b4e80760bdf909))

## [0.7.1](https://github.com/raqeebshafeeque/flowd/compare/v0.7.0...v0.7.1) (2026-11-19)


### Bug Fixes

* **cli:** name the day above every table ([#44](https://github.com/raqeebshafeeque/flowd/issues/44)) ([b093c3c](https://github.com/raqeebshafeeque/flowd/commit/b093c3c81ff0fdf425012c7a3903e4fa1517685b))
* **cli:** show the date above the list table ([#42](https://github.com/raqeebshafeeque/flowd/issues/42)) ([34a85e5](https://github.com/raqeebshafeeque/flowd/commit/34a85e51aff9a35a5f5cb83ecd013d32f6038de3))

## [0.7.0](https://github.com/raqeebshafeeque/flowd/compare/v0.6.0...v0.7.0) (2026-11-19)


### ⚠ BREAKING CHANGES

* show paused time instead of elapsed time ([#41](https://github.com/raqeebshafeeque/flowd/issues/41))

### Features

* **cli:** show items in a table like the workspace ([#39](https://github.com/raqeebshafeeque/flowd/issues/39)) ([20d369a](https://github.com/raqeebshafeeque/flowd/commit/20d369a73e5bcadfd0441bf9216a6695dad798be))
* show paused time instead of elapsed time ([#41](https://github.com/raqeebshafeeque/flowd/issues/41)) ([7c834fa](https://github.com/raqeebshafeeque/flowd/commit/7c834fa016008fa11970f464ac2144db4d60ea10))

## [0.6.0](https://github.com/raqeebshafeeque/flowd/compare/v0.5.0...v0.6.0) (2026-11-19)


### ⚠ BREAKING CHANGES

* **cli:** plain errors for people and options anywhere ([#37](https://github.com/raqeebshafeeque/flowd/issues/37))

### Features

* **cli:** plain errors for people and options anywhere ([#37](https://github.com/raqeebshafeeque/flowd/issues/37)) ([7009cb3](https://github.com/raqeebshafeeque/flowd/commit/7009cb3d30ca698eef9b1f661562093950b28a83))

## [0.5.0](https://github.com/raqeebshafeeque/flowd/compare/v0.4.2...v0.5.0) (2026-11-18)


### ⚠ BREAKING CHANGES

* **cli:** pick items only by their full ID ([#34](https://github.com/raqeebshafeeque/flowd/issues/34))

### Features

* **cli:** pick items only by their full ID ([#34](https://github.com/raqeebshafeeque/flowd/issues/34)) ([8ac5c85](https://github.com/raqeebshafeeque/flowd/commit/8ac5c8514ee8cb753ed3fafb32d4bbddd2540e36))

## [0.4.2](https://github.com/raqeebshafeeque/flowd/compare/v0.4.1...v0.4.2) (2026-11-18)


### Bug Fixes

* accept a relative path in doctor and correct the docs ([#32](https://github.com/raqeebshafeeque/flowd/issues/32)) ([c3f2d8e](https://github.com/raqeebshafeeque/flowd/commit/c3f2d8ef2429aaa6f645e6d6267bb355f4814698))

## [0.4.1](https://github.com/raqeebshafeeque/flowd/compare/v0.4.0...v0.4.1) (2026-11-18)


### Bug Fixes

* **tui:** leave a row between the title and duration fields ([#28](https://github.com/raqeebshafeeque/flowd/issues/28)) ([0389375](https://github.com/raqeebshafeeque/flowd/commit/038937510c593d4ebf71504d6cb37cd01301211b))
* **tui:** polish the list, details and calendar ([#26](https://github.com/raqeebshafeeque/flowd/issues/26)) ([8a7c559](https://github.com/raqeebshafeeque/flowd/commit/8a7c559070be16f0efdd27b9f1c61d8e14694112))

## [0.4.0](https://github.com/raqeebshafeeque/flowd/compare/v0.3.0...v0.4.0) (2026-11-17)


### Features

* **cli:** ask before deleting ([#25](https://github.com/raqeebshafeeque/flowd/issues/25)) ([e823797](https://github.com/raqeebshafeeque/flowd/commit/e82379793c11f5ad05e2c9a8478283e4bf775628))
* **cli:** print third-party licenses with --licenses ([#23](https://github.com/raqeebshafeeque/flowd/issues/23)) ([c1119ed](https://github.com/raqeebshafeeque/flowd/commit/c1119ed5706d6ce94887fed0f5d9d2c1166c6c16))

## [0.3.0](https://github.com/raqeebshafeeque/flowd/compare/v0.2.0...v0.3.0) (2026-11-17)


### Features

* limit Duration to the time left in the day ([#22](https://github.com/raqeebshafeeque/flowd/issues/22)) ([436c535](https://github.com/raqeebshafeeque/flowd/commit/436c535bc0cd0442d3953b28a4d888b8c62aaff6))


### Bug Fixes

* **security:** refuse hidden characters in titles ([#18](https://github.com/raqeebshafeeque/flowd/issues/18)) ([d10e91f](https://github.com/raqeebshafeeque/flowd/commit/d10e91fbb6b29648699af085214d9fb79567f017))


### Performance Improvements

* check integrity only when due and refresh the TUI on change ([#21](https://github.com/raqeebshafeeque/flowd/issues/21)) ([5c64008](https://github.com/raqeebshafeeque/flowd/commit/5c64008488371811ee106e3d9a5aae1acb9702ca))

## [0.2.0](https://github.com/raqeebshafeeque/flowd/compare/v0.1.0...v0.2.0) (2026-11-17)


### Features

* **tui:** add and edit items ([#14](https://github.com/raqeebshafeeque/flowd/issues/14)) ([ad22b23](https://github.com/raqeebshafeeque/flowd/commit/ad22b23f379a005454cf25022c557ee0e2a3ced4))
* **tui:** browse dates with a calendar ([#15](https://github.com/raqeebshafeeque/flowd/issues/15)) ([551adbb](https://github.com/raqeebshafeeque/flowd/commit/551adbbb67c6e8b0047bbe9225b4b55896570b5a))
* **tui:** confirm deletion and add Help ([#17](https://github.com/raqeebshafeeque/flowd/issues/17)) ([61f9a66](https://github.com/raqeebshafeeque/flowd/commit/61f9a66f0e018f28ada3a4c58c5f37b243933f49))
* **tui:** review the day ([#16](https://github.com/raqeebshafeeque/flowd/issues/16)) ([15add4e](https://github.com/raqeebshafeeque/flowd/commit/15add4e0e537126b33829e000831598861daa313))
* **tui:** show the day and its details ([#11](https://github.com/raqeebshafeeque/flowd/issues/11)) ([d48b38b](https://github.com/raqeebshafeeque/flowd/commit/d48b38b85f8710d0adb747609683558836bec3ae))
* **tui:** track work with single keys ([#13](https://github.com/raqeebshafeeque/flowd/issues/13)) ([2752779](https://github.com/raqeebshafeeque/flowd/commit/27527793770cae78a2711119fb3538fdbd9eea68))

## 0.1.0 (2026-11-17)


### Features

* add the Agent Skill ([#8](https://github.com/raqeebshafeeque/flowd/issues/8)) ([e2a06c0](https://github.com/raqeebshafeeque/flowd/commit/e2a06c0e7b6944225c97c8050023976219423c2b))
* **cli:** plan, track and review from the command line ([#7](https://github.com/raqeebshafeeque/flowd/issues/7)) ([f7c9bff](https://github.com/raqeebshafeeque/flowd/commit/f7c9bff83f954dd15610f2e193aa60ce0ff2b351))
* **domain:** close days at their own midnight ([#3](https://github.com/raqeebshafeeque/flowd/issues/3)) ([22a0a81](https://github.com/raqeebshafeeque/flowd/commit/22a0a81927443b3e46e7f1d6506cbdb0ac5873a8))
* **domain:** model daily effort and its lifecycle ([#2](https://github.com/raqeebshafeeque/flowd/issues/2)) ([5e0cfaa](https://github.com/raqeebshafeeque/flowd/commit/5e0cfaa5577c40c55ff550356114b51a9d974185))
* **planner:** share operations across frontends ([#6](https://github.com/raqeebshafeeque/flowd/issues/6)) ([be2ef66](https://github.com/raqeebshafeeque/flowd/commit/be2ef66f30c09de6285009a8dcf0e1c758906b85))
* **store:** add backup, doctor and repair ([#5](https://github.com/raqeebshafeeque/flowd/issues/5)) ([5ab40cb](https://github.com/raqeebshafeeque/flowd/commit/5ab40cb6ba9d39efb57196a226ce4e1e6a7d729b))
* **store:** persist days, items and events in SQLite ([#4](https://github.com/raqeebshafeeque/flowd/issues/4)) ([6adb45a](https://github.com/raqeebshafeeque/flowd/commit/6adb45aa0e2a62ce004bbb2c259a87c857d0faeb))


### Documentation

* plan daily effort instead of time slots ([259eaeb](https://github.com/raqeebshafeeque/flowd/commit/259eaeb56e97faf75488a206d01351b3fea43336))
