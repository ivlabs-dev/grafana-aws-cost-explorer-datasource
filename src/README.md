# AWS Cost Explorer for Grafana

Query AWS Cost Explorer directly from Grafana with a visual query builder and
server-side caching—without building a CUR/Athena pipeline or manually signing
API requests.

This independent open-source backend data-source plugin supports encrypted
static credentials and AssumeRole with explicit source credentials. It queries
`GetCostAndUsage`, follows AWS pagination, and returns native Grafana
time-series or table data frames.

## Highlights

- visual metric, daily/monthly granularity, grouping, filtering, and format
  controls
- up to two Cost Explorer grouping dimensions
- AWS service, linked account, region, and cost allocation tag filters
- bounded 15-minute TTL/LRU cache by default
- one AWS API call for identical concurrent cache misses
- no AWS secrets returned to the browser or logged
- least-privilege target policy: `ce:GetCostAndUsage`

## Configuration

**Static credentials** accepts an access key ID, secret access key, and optional
session token. **Assume an IAM role** requires explicit source credentials and
accepts a target role ARN, optional external ID, and session name. All
credential values are stored only in Grafana secure JSON data; the plugin does
not use the AWS SDK ambient credential chain.

The final AWS identity needs:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "ce:GetCostAndUsage",
      "Resource": "*"
    }
  ]
}
```

The plugin requires no AWS write access. AWS Cost Explorer requests can incur
AWS charges, so successful responses are cached and identical in-flight
requests are deduplicated.

## Documentation

Full installation, IAM, development, security, caching, troubleshooting,
limitations, and contribution documentation is available in the
[GitHub repository](https://github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource).

## Project status

This is an MVP and has not yet had a signed Grafana catalog release.
Screenshots are pending.

This project is not created, sponsored, or endorsed by Amazon Web Services or
Grafana Labs. AWS and Amazon Web Services are trademarks of Amazon.com, Inc. or
its affiliates. Grafana is a trademark of Grafana Labs.

Licensed under Apache License 2.0.
