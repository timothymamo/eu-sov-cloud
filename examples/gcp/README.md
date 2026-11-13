# GCP sample

A made-up web shop on Google Cloud in `europe-west1`, with a read replica in `europe-west4`, as an
inventory: dummy data only, from no real project. The raw types are Cloud Asset Inventory asset
types, as `gcloud asset search-all-resources` and `gcloud asset export` report them.

| ID | Raw type | Category | Capabilities |
|---|---|---|---|
| `shop-web-1` | `compute.googleapis.com/Instance` | `compute.vm` | `ipv6` |
| `shop-web-mig` | `compute.googleapis.com/InstanceGroupManager` | `compute.vm` | `autoscaling`, `zone-high-availability` |
| `shop-web-1-data` | `compute.googleapis.com/Disk` | `storage.block` | `snapshots`, `encryption-at-rest` |
| `shop-assets` | `storage.googleapis.com/Bucket` | `storage.object` | `object-versioning` |
| `shop-db` | `sqladmin.googleapis.com/Instance` | `database.postgresql` | `point-in-time-restore`, `zone-high-availability`, `read-replicas` |
| `shop-db-replica` | `sqladmin.googleapis.com/Instance` | `database.postgresql` | |
| `shop-catalog-mysql` | `sqladmin.googleapis.com/Instance` | `database.mysql` | `point-in-time-restore`, `read-replicas` |
| `shop-gke` | `container.googleapis.com/Cluster` | `kubernetes.managed` | `zone-high-availability`, `autoscaling` |
| `shop-resize-images` | `cloudfunctions.googleapis.com/Function` | `serverless.function` | `autoscaling` |
| `shop-orders` | `pubsub.googleapis.com/Subscription` | `messaging.queue` | `dead-letter-queue` |
| `shop-cdn-backend` | `compute.googleapis.com/BackendBucket` | `unclassified` | |
| `shop-zone` | `dns.googleapis.com/ManagedZone` | `unclassified` | |

Assess it from a clone:

```bash
make build
./bin/shift2eu assess --inventory examples/gcp/inventory.json --target all
```

[Try it](../../docs/guide/try-it.md) walks through the commands and the report with the AWS sample;
every command works with this one too.
