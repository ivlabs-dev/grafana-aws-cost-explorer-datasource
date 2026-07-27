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
optional filters, result format, period controls, and presentation controls.
The backend applies defaults and validates the model independently of the UI.
Queries saved before the period and presentation fields were added keep these
defaults:

- `includeIncompletePeriod=false`
- `topN=0`
- `includeOther=true`
- `alignMonthlyToCalendar=true`
- `rangeMode=dashboard`

Multiple Grafana queries are processed into separate responses keyed by
`refId`; an error in one query does not crash or discard other query responses.

The dashboard time range remains Grafana-owned. The backend converts its UTC
timestamps into Cost Explorer date strings. AWS start dates are inclusive and
end dates are exclusive. A non-midnight upper timestamp is rounded to the next
UTC day so the date containing that endpoint is included. `rangeMode` is
normally `dashboard`; `monthToDate` and `previousEquivalentPeriod` are narrowly
scoped internal modes used by the provisioned KPI panels.

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

1. Decode, default, and validate the versioned query.
2. Resolve the effective UTC billing period and range mode.
3. Exclude the current daily/monthly period unless it was explicitly included.
4. Align monthly starts and ends to calendar boundaries when configured.
5. Return an empty frame and notice without calling AWS if no completed period
   remains.
6. Build a typed `GetCostAndUsageInput`, including AND-combined filters.
7. Hash the safe credential context and upstream request into a cache key.
8. Return an unexpired cache entry, join an identical in-flight request, or
   become the single loader.
9. Call `GetCostAndUsage` until `NextPageToken` is empty.
10. Cache successful combined results; never cache errors.
11. Normalize every paginated result, rank groups across the complete range,
    and optionally aggregate excluded groups as `Other` per period and unit.
12. Convert the prepared results into time-series or table data frames.
13. Attach non-sensitive execution metadata and return frames or a per-query
    actionable error to Grafana.

All AWS work uses the Grafana request context plus a 30-second safety timeout.
Cancellation therefore stops SDK retries and network calls.

### Billing-period resolution

UTC is the billing-period clock because the plugin has no configurable billing
timezone. Daily exclusion stops the exclusive AWS end date at today's UTC
midnight. Monthly exclusion stops it at the first day of the current UTC month.
Yesterday and prior completed months are retained. Including the current period
sets response metadata so callers can distinguish intentional partial data.

Calendar-aligned monthly queries normalize the start to the first day of its
month and use first-of-month end boundaries. Alignment can be disabled to
preserve the range-derived request, but the result can then represent a partial
month. Neither path mutates Grafana's dashboard range.

## Caching

The MVP cache is an in-process TTL/LRU implementation. Default TTL is 15
minutes and default capacity is 256 entries. Configuration is validated to
1-86400 seconds and 1-10000 entries.

Keys include authentication mode, role ARN, region, requested period,
granularity, metric, groupings, and filters. Presentation-only fields—result
format, Top N, and `Other`—are intentionally absent, allowing table and
time-series views to reuse the same fully paginated upstream result. The key
emitted internally is only a SHA-256 digest. Access keys, secret keys, tokens,
and external IDs are never included. Filter values affect the digest but are
not logged.

Per-key in-flight state ensures concurrent identical misses make one AWS API
call. A distributed implementation can later satisfy the same cache interface;
the MVP does not share cache state between Grafana replicas.

## Data-frame generation

For time series, each unique grouping combination becomes a frame with a UTC
time field and a numeric metric field. Timestamps represent the beginning of
AWS billing periods; the backend never adds a local timezone offset. Display
names contain only non-empty human-readable group values, joined by `·` for
two dimensions. Original dimension names and values remain Grafana labels, and
canonical dimension/value identities keep duplicate visible names stable. An
ungrouped frame uses the metric name.

Top N is a post-pagination presentation step. Amount strings are accumulated
with exact rational arithmetic and ranked by total across the complete range;
identity breaks equal-total ties deterministically. Limits are applied per
metric unit. If enabled, `Other` contains one sum for every returned period and
unit, including explicit zeroes where required, and exposes only the safe
`costexplorer_group=Other` label.

For tables, one frame contains `Period`, requested grouping columns, metric,
amount, and unit. `Period` is a sortable `YYYY-MM-DD` or `YYYY-MM` string, which
avoids browser-local rendering of UTC midnight. Empty results still return
typed, zero-row frames so panels remain stable.

## Query execution metadata

Every result frame contains a `costExplorer` object in Grafana frame custom
metadata:

- `queryExecutedAt`
- `awsApiDurationMs`
- `queryDurationMs`
- `cacheStatus`
- `cacheAgeSeconds` for hits
- `cacheTtlSeconds`
- `pageCount`
- `includesIncompletePeriod`
- `containsEstimatedData`

The metadata is suitable for Query Inspector and is not repeated as visible
data columns. Cache hits and shared in-flight results report zero AWS duration
and pages for the current execution. An effective-range bypass reports
`bypass`. AWS errors include an empty typed frame with the safe execution
metadata alongside the per-query error where the SDK response permits it.

## Logging and sensitive data

Structured backend logs use the same safe execution fields plus frame count,
reference ID, and safe data-source context. Metadata and logging are generated
independently. Neither records request objects, filters, tag values, cache keys,
credential objects, access keys, secret keys, session tokens, external IDs,
AssumeRole session details, or signed headers.

## Future extension points

The versioned model can be migrated without silently reinterpreting dashboards.
The AWS and cache interfaces allow mocks, alternate billing clients, and
distributed caching. New frame builders can support forecasts or commitment
data. CUR/Athena, budgets, anomaly detection, Organizations discovery, hosted
services, and commercial features remain deliberately outside the MVP.
