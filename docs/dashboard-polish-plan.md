# Dashboard polish implementation plan

## Objective

Deliver six compatible improvements to the existing AWS Cost Explorer data
source without replacing its current backend-service, cache, frame, or React
editor architecture:

1. incomplete daily/monthly period handling;
2. human-readable grouped-series names;
3. full-range Top N ranking with per-period `Other`;
4. unambiguous table periods and calendar-aligned monthly queries;
5. four summary KPI panels in the provisioned dashboard; and
6. non-sensitive freshness, pagination, estimation, and cache metadata.

The final integration branch is `codex/dashboard-polish-integration`. Every
feature branch starts from the shared-foundation commit on that branch.

## Current architecture assessment

- `pkg/models/query.go` owns the version-1 JSON query contract, validation,
  defaults, and conversion of Grafana timestamps to AWS inclusive-start /
  exclusive-end dates.
- `pkg/costexplorer/service.go` builds `GetCostAndUsage` input, hashes the safe
  credential context and query into a cache key, loads all AWS pages, and
  returns execution timing/cache information.
- `pkg/cache/cache.go` is a bounded in-memory TTL/LRU cache with per-key
  in-flight deduplication. It currently reports hit/miss/shared but not entry
  age.
- `pkg/frames/frames.go` converts the fully paginated AWS response into either
  one frame per time series or a table frame. This is the correct layer for
  post-pagination group ranking and presentation.
- `pkg/plugin/datasource.go` validates each Grafana query, resolves the date
  range, calls the service, converts frames, sets RefIDs, and emits structured
  logs. It is the final owner of frame metadata and notices.
- `src/types.ts`, `src/options.ts`, and
  `src/components/QueryEditor.tsx` contain the matching TypeScript model,
  options, normalization, and editor controls.
- `provisioning/dashboards/json/aws-cost-explorer.json` is the single
  provisioned example dashboard. It currently contains two charts and two
  tables, but no stat panels.
- Existing Go tests mock AWS and cover validation, exclusive end dates,
  pagination, basic frame conversion, cache behavior, and datasource query
  execution. Jest covers the editor defaults and primary controls. Playwright
  smoke tests use `@grafana/plugin-e2e`.

## Shared query contract

The version remains `1`; adding optional fields does not require a migration.
Legacy JSON is normalized to the following defaults:

| JSON field                | Type    | Default     | Notes                                                                 |
| ------------------------- | ------- | ----------- | --------------------------------------------------------------------- |
| `includeIncompletePeriod` | boolean | `false`     | Include the current UTC billing day/month.                            |
| `topN`                    | integer | `0`         | Allowed values: `0`, `5`, `10`, `20`.                                 |
| `includeOther`            | boolean | `true`      | Used only when grouped and `topN > 0`.                                |
| `alignMonthlyToCalendar`  | boolean | `true`      | Used only for monthly queries.                                        |
| `rangeMode`               | string  | `dashboard` | Narrow modes: `dashboard`, `monthToDate`, `previousEquivalentPeriod`. |

Go uses optional boolean pointers for the two true-by-default values so an
absent legacy field can be distinguished from an explicit `false`. Helper
methods expose normalized booleans to feature code. TypeScript normalization
always returns concrete booleans.

## Period and range semantics

- UTC is the billing calendar because the plugin has no configurable billing
  timezone.
- AWS `Start` stays inclusive and `End` stays exclusive.
- Daily queries exclude the current UTC day by capping the exclusive end at
  today's UTC midnight unless `includeIncompletePeriod` is true.
- Monthly calendar alignment moves the start to the first day of its month and
  uses month boundaries. With incomplete periods excluded, the end is capped
  at the first day of the current UTC month.
- A range containing no completed period returns a valid empty frame with an
  informational notice and performs no AWS call.
- `monthToDate` starts at the first day of the month containing the request end
  and intentionally includes incomplete data.
- `previousEquivalentPeriod` derives a preceding range from the normalized
  current billing-period span so comparisons use equivalent completed periods.
- Table frames expose `Period` as `YYYY-MM-DD` or `YYYY-MM`; time-series frames
  continue using UTC timestamps at the beginning of each AWS billing period.

## Group presentation and Top N

- Grouped display names contain only non-empty values joined with ` ·`.
  Dimension/value pairs remain in field labels.
- An ungrouped series retains the selected metric name.
- Series identity is deterministic from dimensions, values, and unit; labels
  preserve uniqueness when visible values collide.
- Ranking happens only after all AWS pages are combined. Totals use
  decimal-safe arithmetic (`math/big.Rat`) before conversion to Grafana's
  numeric field type.
- Equal totals are ordered by a stable canonical group key.
- `Other` is aggregated independently for each billing period and unit. Mixed
  units are never summed together. For metrics with multiple units, Top N is
  selected per unit because cross-unit numeric ranking is misleading.
- The table and time-series paths consume the same normalized/ranked result.

## Freshness and cache metadata

Every returned frame receives `data.FrameMeta.Custom` under
`costExplorer` with:

- `queryExecutedAt`
- `awsApiDurationMs`
- `queryDurationMs`
- `cacheStatus`
- `cacheAgeSeconds` for hits
- `cacheTtlSeconds`
- `pageCount`
- `includesIncompletePeriod`
- `containsEstimatedData`

The same safe values are logged structurally. Cache entries record insertion
time and expose age/TTL through the cache result contract. A shared in-flight
load is normalized to a cache hit-like status without leaking the key. Errors
keep their Grafana per-query error response and structured duration/cache-safe
logging; no credentials, role external IDs, request headers, raw filters, or
tag values enter metadata.

## Dashboard KPI design

Four stat panels occupy the first row:

1. **Cost in selected period** — ungrouped `UnblendedCost`, reduced by sum.
2. **Month-to-date cost** — `rangeMode=monthToDate`,
   `includeIncompletePeriod=true`, reduced by sum, with reporting-delay help.
3. **Change vs previous period** — current and
   `previousEquivalentPeriod` totals with Grafana reduction/math and a
   division-by-zero no-data path; neutral wording avoids treating higher spend
   as good or bad.
4. **Highest-cost service** — `SERVICE`, `topN=1`, `includeOther=false`,
   reduced across the full range so the cleaned series name is primary.

Existing charts move below the KPI row. The service chart uses Top 10 plus
`Other`; the monthly panel remains **Monthly amortized cost** with calendar
alignment enabled.

## File ownership

Shared files are changed only in the foundation phase or by the lead during
integration:

| Owner                    | Files                                                                                                                                                                                                                    |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Shared foundation / lead | `pkg/models/query.go`, `pkg/models/query_test.go`, `src/types.ts`, `src/types.test.ts`, `src/options.ts`                                                                                                                 |
| Lead final wiring        | `pkg/frames/frames.go`, `src/components/QueryEditor.tsx`, `src/components/QueryEditor.test.tsx`, cross-feature tests, `README.md`                                                                                        |
| Period semantics agent   | `pkg/models/period.go`, `pkg/models/period_test.go`, `pkg/frames/period.go`, `pkg/frames/period_test.go`, `src/components/PeriodOptions.tsx`, `src/components/PeriodOptions.test.tsx`                                    |
| Group presentation agent | `pkg/frames/groups.go`, `pkg/frames/groups_test.go`, `src/components/GroupLimitOptions.tsx`, `src/components/GroupLimitOptions.test.tsx`                                                                                 |
| Query metadata agent     | `pkg/cache/cache.go`, `pkg/cache/cache_test.go`, `pkg/costexplorer/service.go`, `pkg/costexplorer/service_test.go`, `pkg/plugin/datasource.go`, `pkg/plugin/datasource_test.go`, optional new metadata helper/test files |
| Dashboard KPI agent      | `provisioning/dashboards/json/aws-cost-explorer.json`, dashboard-only validation/smoke-test files                                                                                                                        |

Agents must not edit `pkg/frames/frames.go` or
`src/components/QueryEditor.tsx`; they expose tested helpers/components for
the lead to wire. If a feature requires a shared-model change beyond the
foundation, the agent reports it rather than modifying the shared files.

## Dependency graph and integration order

```text
assessment + plan
        |
shared query-model foundation
        |
        +--> period semantics -----------+
        +--> group presentation ---------+--> lead wiring and integration tests
        +--> query metadata -------------+
        +--> dashboard KPIs -------------+
                                            |
                          correctness + security/UX reviews
                                            |
                              docs + complete validation
```

Feature branches are integrated in the required order:

1. `codex/period-semantics`
2. `codex/group-presentation`
3. `codex/query-metadata`
4. `codex/dashboard-kpis`

The dashboard agent initially targets only the shared contract and documents
any behavior it needs from other branches.

## Testing strategy

- Model compatibility: legacy Go JSON unmarshal/default tests and TypeScript
  normalization tests.
- Period semantics: injected UTC clock, day/month-only ranges, year/leap-year
  boundaries, empty effective range, and exclusive end dates.
- Presentation: grouped/ungrouped names, two dimensions, missing values,
  duplicate visible names, decimal totals, deterministic ties, unit separation,
  per-period `Other`, tables, and time series.
- Metadata: cache hit/miss/shared status, age, TTL, pagination, estimated data,
  incomplete-period flags, errors, and serialized secret-denylist checks.
- Dashboard: JSON parsing, required panel IDs/titles/layout, expected query
  flags, transformations/expressions, and a local Grafana smoke test when the
  Docker environment is available.
- Final: Prettier check/write as needed, ESLint, Jest, TypeScript, production
  webpack, `gofmt`, `go vet`, Go tests, Go build, Mage packaging, dashboard JSON
  validation, and `@grafana/plugin-e2e`.

## Milestones

1. Commit the compatible query contract and defaults.
2. Complete four isolated feature branches with focused tests.
3. Run two independent read-only reviews.
4. Integrate in order, wiring helpers without whole-side conflict resolution.
5. Add documentation and cross-feature coverage.
6. Run the full validation matrix and leave one verified integration branch.

## Risks and assumptions

- AWS may mark current data as `Estimated`; this is independent of the
  plugin's period-incomplete flag, so both are exposed.
- Grafana frames ultimately store numeric values as floating point. Decimal-safe
  arithmetic is used for ranking and aggregation before the final conversion.
- Query Inspector display of custom frame metadata depends on Grafana version;
  SDK-supported `FrameMeta.Custom` is used without adding repeated columns.
- Grafana stat transformation/expression JSON is version-sensitive. The
  dashboard is validated against the repository's supported Grafana versions
  and falls back to straightforward reductions if secondary text cannot be
  represented portably.
- Real AWS smoke testing stays opt-in and is never required by automated tests.
- The primary checkout contains preserved release/license changes relative to
  its read-only Git index. All dashboard-polish work occurs in clean isolated
  worktrees created from the writable clone; the primary checkout is untouched.
