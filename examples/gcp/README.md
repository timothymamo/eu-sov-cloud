# GCP sample

A made-up web shop on Google Cloud in `europe-west1`, with a read replica in `europe-west4`, as an
inventory: dummy data only, from no real project. The raw types are GCP Config Connector kinds.

| ID | Raw type | Category | Capabilities |
|---|---|---|---|
| `shop-web-1` | `ComputeInstance` | `compute.vm` | `ipv6` |
| `shop-web-mig` | `ComputeRegionInstanceGroupManager` | `compute.vm` | `autoscaling`, `zone-high-availability` |
| `shop-web-1-data` | `ComputeDisk` | `storage.block` | `snapshots`, `encryption-at-rest` |
| `shop-assets` | `StorageBucket` | `storage.object` | `object-versioning` |
| `shop-db` | `SQLInstance` | `database.postgresql` | `point-in-time-restore`, `zone-high-availability`, `read-replicas` |
| `shop-db-replica` | `SQLInstance` | `database.postgresql` | |
| `shop-catalog-mysql` | `SQLInstance` | `database.mysql` | `point-in-time-restore`, `read-replicas` |
| `shop-gke` | `ContainerCluster` | `kubernetes.managed` | `zone-high-availability`, `autoscaling` |
| `shop-resize-images` | `CloudFunctionsFunction` | `serverless.function` | `autoscaling` |
| `shop-orders` | `PubSubSubscription` | `messaging.queue` | `dead-letter-queue` |
| `shop-cdn-backend` | `ComputeBackendBucket` | `unclassified` | |
| `shop-zone` | `DNSManagedZone` | `unclassified` | |

Assess it from a clone:

```bash
make build
./bin/shift2eu assess --inventory examples/gcp/inventory.json --target all
```

[Try it](../../docs/guide/try-it.md) walks through the commands and the report with the AWS sample;
every command works with this one too.
