# Changelog

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
