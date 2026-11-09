# Shared test fixtures

Fixtures used by more than one package or by the e2e tests: sample inventories, recorded provider API
responses and catalog snapshots. Fixtures used by a single package live in that package's own
`testdata/` directory.

Every fixture is **sanitised** before it is committed: no real account, subscription, tenant or project
IDs, ARNs, IP addresses, hostnames, tags or names. Use documentation values such as `123456789012`,
`192.0.2.0/24` and `example.com`.
