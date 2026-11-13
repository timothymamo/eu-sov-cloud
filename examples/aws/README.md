# AWS sample

A made-up web shop on AWS in `eu-central-1`, as an inventory: dummy data only, from no real account.

| ID | Raw type | Category | Capabilities |
|---|---|---|---|
| `shop-web-1`, `shop-web-2` | `AWS::EC2::Instance` | `compute.vm` | `ipv6`, `snapshots` |
| `shop-web-asg` | `AWS::AutoScaling::AutoScalingGroup` | `compute.vm` | `autoscaling`, `zone-high-availability` |
| `shop-web-1-data` | `AWS::EC2::Volume` | `storage.block` | `encryption-at-rest`, `snapshots` |
| `shop-assets` | `AWS::S3::Bucket` | `storage.object` | `object-versioning`, `encryption-at-rest` |
| `shop-db` | `AWS::RDS::DBInstance` | `database.postgresql` | `point-in-time-restore`, `read-replicas`, `zone-high-availability`, `encryption-at-rest` |
| `shop-legacy-mysql` | `AWS::RDS::DBInstance` | `database.mysql` | `point-in-time-restore`, `node-high-availability` |
| `shop-eks` | `AWS::EKS::Cluster` | `kubernetes.managed` | `node-high-availability`, `autoscaling` |
| `shop-thumbnailer` | `AWS::Lambda::Function` | `serverless.function` | |
| `shop-orders` | `AWS::SQS::Queue` | `messaging.queue` | `dead-letter-queue`, `encryption-at-rest` |
| `shop-orders-dlq` | `AWS::SQS::Queue` | `messaging.queue` | `encryption-at-rest` |
| `shop-cdn` | `AWS::CloudFront::Distribution` | `unclassified` | |
| `shop.example.com` | `AWS::Route53::HostedZone` | `unclassified` | |

Assess it from a clone:

```bash
make build
./bin/shift2eu assess --inventory examples/aws/inventory.json --target scaleway
./bin/shift2eu assess --inventory examples/aws/inventory.json --target all
```

This is the sample [Try it](../../docs/guide/try-it.md) uses; its reports are pinned in
[`test/examples/testdata/`](../../test/examples/testdata/).
