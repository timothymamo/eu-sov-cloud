# Azure sample

A made-up web shop on Azure in `westeurope`, as an inventory: dummy data only, from no real
subscription. The raw types are Azure Resource Manager resource types.

| ID | Raw type | Category | Capabilities |
|---|---|---|---|
| `vm-shop-web-1` | `Microsoft.Compute/virtualMachines` | `compute.vm` | `ipv6`, `snapshots` |
| `vmss-shop-web` | `Microsoft.Compute/virtualMachineScaleSets` | `compute.vm` | `autoscaling`, `zone-high-availability` |
| `disk-shop-web-1-data` | `Microsoft.Compute/disks` | `storage.block` | `encryption-at-rest`, `snapshots` |
| `stshopassets` | `Microsoft.Storage/storageAccounts` | `storage.object` | `object-versioning`, `encryption-at-rest` |
| `psql-shop` | `Microsoft.DBforPostgreSQL/flexibleServers` | `database.postgresql` | `point-in-time-restore`, `zone-high-availability`, `read-replicas` |
| `mysql-shop-catalog` | `Microsoft.DBforMySQL/flexibleServers` | `database.mysql` | `point-in-time-restore`, `node-high-availability`, `read-replicas` |
| `aks-shop` | `Microsoft.ContainerService/managedClusters` | `kubernetes.managed` | `zone-high-availability`, `autoscaling` |
| `func-shop-thumbnails` | `Microsoft.Web/sites` | `serverless.function` | `ipv6` |
| `sb-shop-orders` | `Microsoft.ServiceBus/namespaces/queues` | `messaging.queue` | `dead-letter-queue` |
| `cdn-shop` | `Microsoft.Cdn/profiles` | `unclassified` | |
| `dns-shop` | `Microsoft.Network/dnsZones` | `unclassified` | |

Assess it from a clone:

```bash
make build
./bin/shift2eu assess --inventory examples/azure/inventory.json --target all
```

[Try it](../../docs/guide/try-it.md) walks through the commands and the report with the AWS sample;
every command works with this one too.
