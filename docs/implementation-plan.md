# AWS Cost Explorer for Grafana implementation plan

## Objective

Deliver an open-source Grafana backend data-source plugin that queries AWS Cost
Explorer through a visual editor, keeps AWS credentials on the Grafana server,
returns native Grafana data frames, and avoids duplicate or unnecessarily
frequent paid Cost Explorer requests.

Temporary plugin ID: `ivlabsdev-awscostexplorer-datasource`. Grafana's
scaffolder normalizes the GitHub organization name `ivlabs-dev` to the plugin
ID prefix `ivlabsdev`.

The plugin is an independent community project. It is not created, sponsored,
or endorsed by Amazon Web Services or Grafana Labs.

## Verified platform baseline

The repository is scaffolded with `@grafana/create-plugin` 7.9.0, the current
official Grafana scaffolder checked on 2026-07-26. The generated baseline uses
Grafana frontend packages 13.1.x, Grafana Plugin SDK for Go 0.294.0, webpack,
Jest, Playwright through `@grafana/plugin-e2e`, Mage backend builds, and the
standard Docker development environment.

The AWS implementation uses AWS SDK for Go v2. `config.LoadDefaultConfig`
provides the supported default credential chain, and `stscreds` provides
AssumeRole credentials. Cost Explorer requests use `GetCostAndUsage`; its start
date is inclusive, its end date is exclusive, and it accepts at most two
groupings.

## Architecture

The frontend sends a versioned JSON query model through Grafana's normal
`DataSourceWithBackend` query path. Grafana creates one backend instance for
each configured data-source instance. That instance owns an AWS client and a
bounded in-memory cache, which prevents credentials, clients, and cached data
from crossing data-source boundaries.

Backend packages:

- `pkg/models`: settings, secure settings, versioned query model, validation,
  and Grafana-to-Cost-Explorer time conversion.
- `pkg/awsclient`: AWS SDK v2 configuration, default/static/AssumeRole
  credential providers, credential resolution, and a mockable Cost Explorer
  client interface.
- `pkg/costexplorer`: safe request construction, filter/group mapping,
  pagination, cache-key generation, AWS error classification, and execution.
- `pkg/cache`: a bounded TTL/LRU cache with an interface and per-key in-flight
  request deduplication.
- `pkg/frames`: conversion of grouped and ungrouped AWS responses to Grafana
  time-series and table data frames.
- `pkg/plugin`: Grafana instance lifecycle, `QueryData`, `CheckHealth`,
  timeouts, and structured logging.

Frontend modules:

- `src/types.ts`: query and configuration types aligned with Go models.
- `src/components/ConfigEditor.tsx`: authentication, region, AssumeRole,
  secure static credentials, and cache controls.
- `src/components/QueryEditor.tsx`: metric, granularity, up to two groupings,
  filters, and result format.
- Validation and option modules keep UI behavior testable and selectors
  reusable.

## Query model

Version 1 contains:

```text
version
metric
granularity
groupBy[0..2]
filter.service
filter.linkedAccount
filter.region
filter.tagKey
filter.tagValue
format
```

The backend owns validation and defaulting even when the frontend already
validates. Grafana's UTC time range is converted to an inclusive start date and
exclusive end date. A non-midnight upper bound is rounded up to the following
UTC day so the date containing the dashboard endpoint is included.

## Authentication and secrets

`default` uses the AWS SDK default credential chain, including environment,
shared files, ECS/EC2 roles, and web identity such as IRSA.

`assumeRole` first loads the default chain, then uses STS AssumeRole with a role
ARN and optional external ID and role session name.

`static` uses an access key ID, secret access key, and optional session token.
All three values, plus the optional external ID, are stored only in Grafana
`secureJsonData`. They are exposed to the backend through
`DecryptedSecureJSONData`, never returned to the browser after saving, never
placed in cache keys, and never logged. IAM/workload roles are the recommended
production mode.

`CheckHealth` validates settings, explicitly resolves credentials, and performs
a minimal one-day `GetCostAndUsage` request. Errors distinguish configuration,
credential resolution, AssumeRole/access denial, request validation,
throttling, and other AWS failures.

## Caching

Each data-source instance has a TTL/LRU cache with a default TTL of 15 minutes
and a default maximum of 256 entries. Both are configurable with validated
bounds.

The key is a SHA-256 digest of a canonical representation containing the safe
credential context (data-source isolation, auth mode, role ARN, and region) and
the complete Cost Explorer request (period, granularity, metric, grouping, and
filters). Secrets are excluded. Filter and tag values influence the digest but
are not emitted in logs. Concurrent misses for the same key share one loader
call. Failed requests are not cached.

The cache interface is deliberately process-local but can later be implemented
by Redis or another distributed store without changing query execution.

## Data frames

Time-series output returns one frame per grouping combination and one numeric
field for the selected metric. Grouping values are labels, period starts are
UTC timestamps, and the AWS unit is attached when consistent.

Table output returns period, grouping dimensions, metric, amount, and unit
columns. Empty AWS responses return schema-correct empty frames rather than
errors.

## Testing strategy

Go tests use mocked Cost Explorer clients and no AWS credentials. Coverage
targets validation, exclusive end dates, pagination, request/error handling,
grouped/ungrouped/empty frames, cache hit/miss/expiry/eviction, concurrent
deduplication, and authentication configuration.

Jest and Testing Library cover query defaults and editor interactions,
configuration validation, and secure field reset/preservation. Playwright
smoke tests remain repeatable against the local Grafana container; real AWS
integration tests are optional and disabled by default.

CI installs locked npm dependencies, type-checks, lints, runs frontend tests,
builds webpack output, checks Go formatting without rewriting it, runs `go
vet`, Go tests, Mage multi-platform builds, and packages the plugin without AWS
credentials.

## Milestones

1. Scaffold and metadata: current Grafana baseline, plugin identity, license,
   Docker development environment, CI, and implementation plan.
2. Working backend slice: secure settings, AWS client factory, health check,
   one metric, daily/monthly requests, pagination, service/account grouping,
   native frames, and cache.
3. Working frontend slice: authentication configuration and visual query
   editor for the first slice.
4. Complete MVP controls: all requested metrics/groupings/filters and two
   grouping dimensions.
5. Quality and distribution: tests, provisioning, dashboard, security and
   architecture docs, packaging, smoke-test guide, and release hardening.

## Risks and assumptions

- Cost Explorer is a billed API and billing data is delayed. Health checks use
  the same cache path to reduce repeat requests, and documentation must set
  expectations about AWS charges and freshness.
- Cost Explorer has no credential-only operation under the minimal
  `ce:GetCostAndUsage` policy. The health check therefore makes a small
  `GetCostAndUsage` call rather than adding STS permissions solely for health.
- Cache state is local to a Grafana plugin process. Multiple Grafana replicas
  do not share entries in the MVP.
- The requested region controls the AWS endpoint/signing configuration. Cost
  Explorer availability and endpoint behavior remain subject to AWS support.
- `UsageQuantity` can mix incompatible units unless the query is filtered to a
  meaningful usage type; the UI and documentation warn about this.
- Static credentials are supported for compatibility, but long-lived keys are
  intentionally discouraged.
- Grafana plugin signing requires an external Grafana access-policy token.
  Local development can explicitly allow this unsigned plugin; CI must not
  require a signing secret.
- The current official scaffold targets Go 1.26.5 while the initial local shell
  reports Go 1.23.5. Verification may require Go's toolchain download or a
  local toolchain upgrade; this is an environment concern, not a reason to
  downgrade the generated supported baseline.

## Future extension points, not MVP work

The query model version, AWS client interfaces, cache interface, and frame
builders leave room for CUR/Athena, budgets, anomaly detection, commitments,
cost categories, Organizations discovery, distributed caching, and managed
services. None of those APIs or product surfaces are implemented in this
slice.
