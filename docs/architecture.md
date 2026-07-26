# Architecture

## System boundary

AWS Cost Explorer for Grafana is a backend data-source plugin. The React query
editor runs in the Grafana browser application, but it never calls AWS. Grafana
sends the versioned query JSON and dashboard time range to the Go plugin
process over the standard backend plugin protocol. Only that server-side
process resolves credentials and sends signed HTTPS requests to AWS.

```text
Grafana browser
  -> Grafana server
    -> plugin backend instance
      -> in-memory cache
        -> AWS SDK v2
          -> AWS Cost Explorer GetCostAndUsage
```

Grafana's instance manager creates a separate `Datasource` object for every
configured data-source instance and replaces it when configuration changes.
Each object owns its AWS client and cache. This provides a natural boundary
between configured roles/accounts and reuses AWS transports and rotating
credential providers safely.

## Frontend/backend communication

`DataSourceWithBackend` sends `CostQuery` as the JSON body of each Grafana
`DataQuery`. Version 1 contains the metric, granularity, up to two groupings,
optional filters, and result format. The backend applies defaults and validates
the model independently of the UI. Multiple Grafana queries are processed into
separate responses keyed by `refId`; an error in one query does not crash or
discard other query responses.

The dashboard time range remains Grafana-owned. The backend converts its UTC
timestamps into Cost Explorer date strings. AWS start dates are inclusive and
end dates are exclusive. A non-midnight upper timestamp is rounded to the next
UTC day so the date containing that endpoint is included.

## Credential handling

Non-sensitive settings live in Grafana `jsonData`: authentication mode, region,
role ARN, role session name, and cache controls.

Sensitive settings live only in `secureJsonData`: external ID, access key ID,
secret access key, and session token. Grafana encrypts these values at rest and
only supplies decrypted values to the backend. After saving, the browser sees
only booleans in `secureJsonFields`, not the stored values.

### Default chain

The backend calls AWS SDK v2 `config.LoadDefaultConfig`. The SDK can resolve
environment credentials, web identity, shared configuration/credentials
files, ECS task roles, and EC2 instance roles. Credential rotation is handled
by the SDK.

### AssumeRole

The backend loads the default chain for source credentials, creates an STS
client, and wraps `stscreds.NewAssumeRoleProvider` in the SDK credential cache.
The configured role ARN, optional external ID, and role session name are sent
to STS. Resulting temporary credentials are held by the SDK and never exposed
to the plugin frontend or query model.

### Static credentials

Static credentials are passed directly to the SDK static provider. They are
supported as a fallback but are intentionally discouraged because they do not
rotate automatically.

## Query lifecycle

1. Decode and validate the versioned query.
2. Convert the Grafana timestamp range to inclusive/exclusive AWS dates.
3. Build a typed `GetCostAndUsageInput`, including AND-combined filters.
4. Hash the safe credential context and canonical request into a cache key.
5. Return an unexpired cache entry, join an identical in-flight request, or
   become the single loader.
6. Call `GetCostAndUsage` until `NextPageToken` is empty.
7. Cache successful combined results; never cache errors.
8. Convert results into time-series or table data frames.
9. Return frames or a per-query actionable error to Grafana.

All AWS work uses the Grafana request context plus a 30-second safety timeout.
Cancellation therefore stops SDK retries and network calls.

## Caching

The MVP cache is an in-process TTL/LRU implementation. Default TTL is 15
minutes and default capacity is 256 entries. Configuration is validated to
1-86400 seconds and 1-10000 entries.

Keys include authentication mode, role ARN, region, requested period,
granularity, metric, groupings, and filters. The key emitted internally is only
a SHA-256 digest. Access keys, secret keys, tokens, and external IDs are never
included. Filter values affect the digest but are not logged.

Per-key in-flight state ensures concurrent identical misses make one AWS API
call. A distributed implementation can later satisfy the same cache interface;
the MVP does not share cache state between Grafana replicas.

## Data-frame generation

For time series, each unique grouping combination becomes a frame with a UTC
time field and a numeric metric field. Group values are Grafana labels. A
consistent three-letter currency unit is mapped to Grafana's currency unit ID.

For tables, one frame contains time period, requested grouping columns, metric,
amount, and unit. Empty results still return typed, zero-row frames so panels
remain stable.

## Logging and sensitive data

Structured backend logs record query duration, AWS duration, cache result,
pagination count, frame count, reference ID, and safe data-source context.
They do not record request objects, filters, tag values, credential objects,
access keys, secret keys, session tokens, or signed headers.

## Future extension points

The versioned model can be migrated without silently reinterpreting dashboards.
The AWS and cache interfaces allow mocks, alternate billing clients, and
distributed caching. New frame builders can support forecasts or commitment
data. CUR/Athena, budgets, anomaly detection, Organizations discovery, hosted
services, and commercial features remain deliberately outside the MVP.
