# Changelog

## Unreleased

- Make the **Mock data** mode's synthetic data realistic: per-service growth
  trends, weekday/weekend and slow monthly patterns, and occasional spikes
  replace the previous flat noise. Totals, per-service, per-account,
  per-region, and per-tag groupings now come from one cost model and
  reconcile exactly; tax is a fixed share of the other services; monthly
  buckets sum their days (and a partial month reads as month-to-date);
  values depend only on the calendar date, never on the requested range; and
  the reservation and Savings Plans utilization/coverage series vary like
  real ones.
- Replace the redacted screenshots with ones captured against the mock data,
  and add screenshots of the Reservation utilization query editor and the
  data source configuration page.

## 1.1.0 (2026-10-28)

- Add a **Mock data** authentication mode needing no AWS account,
  credentials, or IAM permissions at all. Every query type this plugin
  supports -- Cost and usage, Reservation/Savings Plans utilization,
  coverage, and inventory -- returns realistic synthetic data that respects
  the actual requested date range, granularity, and grouping, via a new
  `pkg/awsmock` package implementing the same client interfaces real AWS
  SDK clients do. `CheckHealth` and every query type's existing dispatch
  logic run completely unchanged against it; `SDKFactory.New` is the only
  place that knows mock mode exists. Intended for evaluating the plugin,
  demoing dashboards, or letting someone verify the plugin works without
  sharing real AWS credentials -- for example, a Grafana plugin catalog
  reviewer with no AWS account of their own.
- Bundle the "AWS Cost Explorer overview" dashboard with the plugin itself,
  importable from the data source's **Dashboards** tab. It replaces the
  highest-cost-service stat with a top-10 services bar chart and splits tax
  onto its own axis in the service breakdown.
- Update the local development provisioning dashboard to match.
- Add autocomplete suggestions to the query editor's AWS service, linked
  account, region, tag key, and tag value filter fields, backed by AWS Cost
  Explorer's `GetDimensionValues`/`GetTags` APIs. Free-text entry still works
  as a fallback, including for data sources without the two new optional IAM
  actions (`ce:GetDimensionValues`, `ce:GetTags`).
- Add **Reservation utilization** and **Savings Plans utilization** query
  types (`ce:GetReservationUtilization`/`ce:GetSavingsPlansUtilization`,
  both optional and additive), with table and time-series presentation of a
  curated metric set, an optional group-by-subscription-ID toggle for
  Reservation utilization, and four new bundled dashboard panels. The
  Reservation utilization AWS service field is a required, fixed dropdown of
  the five values AWS actually accepts (EC2/RDS/ElastiCache/Redshift/
  OpenSearch) rather than free text, since AWS silently assumes EC2 if left
  unset — a real account with only RDS reservations previously looked like
  it had none at all. An account/period with no eligible usage now shows a
  normal empty result instead of AWS's `DataUnavailableException` error.
  Reservation/Savings Plans **coverage** is a planned follow-up, not part of
  this change.
- Replace the bundled dashboard's Reservation/Savings Plans utilization stat
  and per-day detail-table panels with utilization-percentage **trend
  charts** (same data, just a different visualization) plus two new
  **inventory** table panels, "Reservations" and "Savings Plans", listing
  what the account currently owns with a computed days-until-expiry column
  -- intended to make it easy to spot a Reservation or Savings Plan that
  needs renewing (alerting on this is a follow-up, not part of this change).
  Add matching **Reservation inventory** and **Savings Plans inventory**
  query types. Unlike every other query type this plugin supports, these
  reflect current state and ignore the dashboard's selected time range.
  Savings Plans inventory uses the AWS Savings Plans service's
  `DescribeSavingsPlans` (`savingsplans:DescribeSavingsPlans`, optional).
  Reservation inventory merges one AWS "Describe" call per
  reservation-eligible service -- EC2, RDS, ElastiCache, Redshift, and
  OpenSearch (`ec2:DescribeReservedInstances`,
  `rds:DescribeReservedDBInstances`, `elasticache:DescribeReservedCacheNodes`,
  `redshift:DescribeReservedNodes`, `es:DescribeReservedInstances`, all
  optional and independent of each other) -- excluding any service whose
  permission wasn't granted from the merged list rather than failing the
  whole query, with a warning notice for any other per-service failure.
- Fix the query editor's `filterQuery` silently dropping the new inventory
  query types' requests before they ever reached the backend -- it
  unconditionally required `granularity`/`format` to be set, which the
  bundled dashboard's inventory panels never set (snapshot queries don't
  have a granularity or format). Same regression class the utilization
  query types hit earlier in this file; now guarded with a test for all
  four non-period-based query types.
- Add **Reservation coverage** and **Savings Plans coverage** query types
  (`ce:GetReservationCoverage`/`ce:GetSavingsPlansCoverage`, both optional
  and additive) -- how much of your eligible usage is covered by what you
  bought, as opposed to Utilization's how much of what you bought you're
  using. Table and time-series presentation of a curated metric set, reusing
  Utilization's presentation code. Reservation coverage's AWS service field
  is a required, fixed dropdown like Reservation utilization's, but with a
  narrower four-value list (EC2/RDS/ElastiCache/Redshift) -- AWS's own
  `GetReservationCoverage` documentation doesn't mention OpenSearch at all,
  unlike `GetReservationUtilization`. Six new bundled dashboard panels: a
  coverage-percentage trend chart plus "On-Demand cost not covered" and
  "Average coverage %" stat panels (computed by Grafana's own sum/mean
  reduce calcs, no new backend aggregation) for both Reservation and Savings
  Plans coverage. AWS's coverage percentage fields come back with an
  embedded `%` suffix (unlike Utilization's plain-number fields); this is
  stripped before parsing. Neither coverage query type supports grouping.
  AWS's "Potential monthly savings vs On-Demand" figure (shown in its own
  Coverage report) is sourced from the separate purchase-recommendation
  APIs, not coverage data -- not part of this change, tracked on the
  roadmap.
- Fix Reservation inventory silently treating "no IAM permission for this
  service" the same as "genuinely no reservations" -- a service whose
  Describe permission wasn't granted is still excluded from the merged
  list, but now with an Info notice naming the specific action to grant,
  instead of no notice at all. Document that Reservation inventory is
  scoped to the data source's configured AWS region (unlike every other
  query type), since EC2/RDS/ElastiCache/Redshift/OpenSearch's Describe
  calls are regional AWS APIs, not account-wide like Cost Explorer and
  Savings Plans.
- Add a "Filter data by values" transformation to the bundled dashboard's
  **Reservations** and **Savings Plans** panels that excludes `retired`
  rows by default, so expired/returned items don't clutter the default
  view. The backend still returns every state AWS reports; remove or edit
  the transformation to see retired ones.
- Switch the bundled dashboard's Reservation utilization/coverage panels'
  default AWS service from EC2 to RDS, since RDS (along with ElastiCache,
  Redshift, and OpenSearch) has no Savings Plans equivalent at all -- unlike
  EC2, Reserved Instances are the only commitment-discount AWS offers for
  those services. Duplicate the relevant panel and change its query
  editor's AWS service field to see a different service's reservations
  instead; a dashboard-variable-based approach was tried and reverted in
  favor of this simpler, explicit default.
- Fix the Reservation/Savings Plans utilization and coverage trend charts'
  Y-axis auto-scaling to 0-200% even though the underlying percentage can
  never exceed 100%, wasting half the chart as dead space and making a
  real, meaningful dip hard to see. Pin the axis to 0-100%.
- Combine the bundled dashboard's four separate Reservation/Savings Plans
  utilization and coverage trend charts into two panels, each plotting
  Utilization % and Coverage % together as two lines over the same range --
  the gap between them is the actionable signal (utilization near 100% with
  coverage well below it means there's room to buy more commitment and
  save further), which was hard to read across four separate panels. Also
  remove the four "On-Demand cost not covered"/"Average coverage %" stat
  panels now that both lines are visible directly on the combined charts.
  Bundled dashboard panel count drops from eighteen to twelve.
- Rename `docs/iam-policy.json` to `docs/iam-policy-full.json` and add three
  scoped alternatives --
  `docs/iam-policy-cost-and-usage-only.json`,
  `docs/iam-policy-reservations-only.json`, and
  `docs/iam-policy-savings-plans-only.json` -- for granting only what a
  given account actually uses, instead of the full permission set. Each
  scoped policy still includes the required `ce:GetCostAndUsage` baseline
  (nothing works without it) but otherwise only the actions for its own
  feature set; autocomplete (`ce:GetDimensionValues`/`ce:GetTags`) is a
  separate, unrelated feature and isn't bundled into the
  Reservations/Savings-Plans-scoped files. Make explicit in the README's
  IAM section that each optional action is required to see that specific
  feature's data (not merely "nice to have"), and that a missing permission
  always resolves to a scoped error on that one panel or query type --
  never a crash, and never something that affects other panels.
- Split the README's deep per-query-type reference material (period
  semantics, and the full Reservation/Savings Plans utilization, inventory,
  and coverage behavior) out into `docs/query-types.md`, and condense the
  IAM policy section into a compact table. README drops from 816 to ~530
  lines; no content removed, just relocated so the landing page stays
  scannable.

## 1.0.8 (2026-09-19)

- First signed release. Enable plugin signing in the release workflow using a
  Grafana Cloud access policy token. No functional changes since 1.0.7.

## 1.0.7 (2026-09-09)

- Rename an internal validation label to clear two false-positive gosec G101
  "hardcoded credentials" findings.
- Raise the Go toolchain to 1.26.8, resolving eight standard-library
  vulnerabilities reported by govulncheck.
- Update `@grafana/plugin-e2e` to 3.12.0 for compatibility with the Grafana
  13.2+ panel editor.
- Bump `google.golang.org/grpc` to 1.83.2 and pin `fast-uri` and `js-yaml`
  to patched versions, clearing HIGH advisories reported by the plugin
  validator's vulnerability scan.

## 1.0.6 (2026-08-20)

- Remove the AWS SDK default credential-chain authentication mode.
- Require explicitly configured source credentials for AssumeRole.
- Return generic health-check failures to the browser while retaining
  diagnostic errors in backend logs.
- Update vulnerable transitive frontend dependencies.

## 1.0.0 (Unreleased)

- Add AWS SDK v2 default, AssumeRole, and secure static authentication.
- Add Cost Explorer health checks and paginated `GetCostAndUsage` queries.
- Add the visual metric, granularity, grouping, filtering, and format editor.
- Add native Grafana time-series and table frames.
- Add bounded TTL/LRU caching with in-flight request deduplication.
- Add mocked backend/frontend tests, provisioning, example dashboard, CI, and
  security documentation.
