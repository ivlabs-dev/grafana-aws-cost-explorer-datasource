# Changelog

## Unreleased

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
