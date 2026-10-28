# Query types reference

Detailed reference for every query type this plugin supports beyond the
basics covered in the main [README](../README.md#query-examples): how
**Cost and usage** periods are presented, and the full behavior of the
**Reservation/Savings Plans utilization, inventory, and coverage** query
types. See the [IAM policy](../README.md#iam-policy) section for which AWS
permission each of these needs.

## Period and presentation semantics

AWS may revise the current day or month while billing data is still arriving.
The plugin therefore excludes the current UTC billing period by default:

- daily queries stop before the current UTC calendar day
- monthly queries stop before the current UTC calendar month
- a range containing no completed periods returns an empty, valid frame with a
  notice and makes no AWS request

Select **Include incomplete current period** when intentionally viewing
month-to-date or current-day data. The frame metadata then records that the
response may contain an incomplete period. This option affects only the Cost
Explorer query; it does not alter the dashboard time range.

Monthly queries select **Align to complete calendar months** by default. The
start is normalized to the first day of its month, and the exclusive end uses
a calendar-month boundary. Disabling alignment preserves the range-derived
behavior and can return a partial first or last month.

Table periods are sortable strings: `YYYY-MM-DD` for daily data and `YYYY-MM`
for monthly data. Time-series values remain Grafana timestamps at the beginning
of the AWS billing period in UTC; the plugin never applies a browser-local
offset.

Grouped time-series display names contain values only, such as `Amazon EC2` or
`Amazon EC2 · Production`. Dimension names and values remain available as
Grafana field labels. Top N ranking uses each group's decimal-safe total across
the complete returned range. When enabled, `Other` sums the excluded groups
separately for every period and never combines incompatible units.

## Reservation and Savings Plans utilization

Choose **Reservation utilization** or **Savings Plans utilization** as the
**Query type** to query AWS's `GetReservationUtilization` and
`GetSavingsPlansUtilization` APIs instead of `GetCostAndUsage`. These are
separate AWS APIs with their own request/response shapes, so the editor
reflects their real constraints rather than pretending a single unified
model:

- Both support **Linked account** and **Region** filters. **AWS service** is
  additionally available for Reservation utilization only — Savings Plans
  utilization doesn't support a service dimension. Neither API supports cost
  allocation tag filtering.
- **Reservation utilization's AWS service field is a required, fixed
  dropdown** of the only five values AWS accepts for this API: EC2, RDS,
  ElastiCache, Redshift, and OpenSearch (Elasticsearch). This is
  deliberate: AWS reports utilization for only **one** of these services per
  request, and **silently assumes EC2 if the filter is left unset** — an
  account with real, active RDS (or ElastiCache/Redshift/OpenSearch)
  reservations and zero EC2 reservations would otherwise show "no data" with
  no indication why. To see more than one service's reservations, duplicate
  the panel/query and change this field.
- Reservation utilization can optionally **Group by subscription ID** (the
  only grouping AWS allows for this API). Savings Plans utilization never
  groups by any dimension.
- **Table** format shows a curated set of AWS's numeric fields as columns:
  utilization %, net RI savings, On-Demand cost of RI hours used, total
  amortized fee, unused hours, and realized/unrealized savings for
  Reservations; utilization %, used/unused/total commitment, net savings,
  On-Demand cost equivalent, and total amortized commitment for Savings
  Plans. AWS returns more fields than this; the list is an intentional,
  curated subset rather than everything AWS exposes.
- **Time series** format plots one metric at a time, chosen via the
  **Utilization metric** selector, from that same curated set.
- An account/period with genuinely no eligible usage for either API is shown
  as a normal empty result, not an error — AWS's `DataUnavailableException`
  for this case is treated the same as a successful empty response.

### Multi-account (AWS Organizations) visibility

Both APIs are scoped to **whichever AWS identity the data source's
credentials resolve to** — this is AWS's own behavior, not something the
plugin controls. A member account only ever sees its own share of a shared
Savings Plan or Reservation; only the management/payer account identity sees
the full picture across all linked accounts (AWS's docs put it as
"Management account in an organization have access to member accounts").

A common setup — a per-environment Grafana (e.g. in a "prod" account) that
also wants to see org-wide utilization for a Savings Plan or Reservation
purchased by the management account — has two options, and doesn't need
anything new from this plugin:

- **Two data sources**: one using the member account's own credentials for
  cost-and-usage, one using management-account credentials (or an
  AssumeRole role defined _in_ the management account, trusting the member
  account) for the utilization query types. Different panels on the same
  dashboard can reference different data sources.
- **One data source** pointed at the management account, since it can see
  cost-and-usage broken down by `LINKED_ACCOUNT` too (see the bundled "Cost
  by linked account" panel) — trading a single, broader-scoped credential
  for simplicity.

## Reservation and Savings Plans inventory

Choose **Reservation inventory** or **Savings Plans inventory** as the
**Query type** to list what you actually own, rather than how well it's
being utilized. These query types are deliberately different from every
other query type this plugin supports in one important way: **they reflect
current state and ignore the dashboard's selected time range entirely.**
"What Reservations do I have right now" isn't a question with a meaningful
historical answer the way "what did I spend last month" is, so there's no
Granularity, Format, or filter control to set — the query editor shows only
the query type selector and a reminder of this behavior.

- **Savings Plans inventory** calls the AWS Savings Plans service's
  `DescribeSavingsPlans` (a different AWS service/API from Cost Explorer
  entirely) and returns one row per Savings Plan: ID, type, payment option,
  region, hourly commitment, state, start date, end date, and a computed
  **days until expiry** (negative once a plan has expired).
- **Reservation inventory** calls one AWS "Describe reserved instances/nodes"
  API per reservation-eligible service — EC2, RDS, ElastiCache, Redshift, and
  OpenSearch — and merges all five into a single list with the same shape:
  service, ID, instance/node type, count, offering type, state, start date,
  computed end date, and days until expiry. Four of the five underlying AWS
  APIs don't return an expiry date directly, only a start time and a
  duration, so the end date is always computed (`start + duration`) rather
  than trusted as a literal AWS field.
- Both tables sort by days-until-expiry ascending, so anything expiring soon
  surfaces first — intended to make it easy to spot a Reservation or Savings
  Plan that needs renewing before it lapses. (Alerting on this threshold is
  on the [Roadmap](../README.md#roadmap), not built yet.)
- The five Reservation inventory permissions are independent of each other.
  A service whose IAM permission wasn't granted is excluded from the merged
  list with an **Info** notice naming it and the specific action to grant,
  visible in the panel and Query Inspector — so "zero reservations" and
  "this service's permission isn't granted" are never indistinguishable. A
  service that fails for some other reason (throttling, a transient AWS
  error) is also excluded, with a **Warning** notice instead. If every one
  of the five services fails for a non-permission reason, the whole query
  fails rather than silently showing an empty, misleading table.
- **Reservation inventory is scoped to the data source's configured AWS
  region**, unlike every other query type in this plugin. Cost Explorer and
  the Savings Plans service are account-wide regardless of which region
  endpoint a request hits, but EC2/RDS/ElastiCache/Redshift/OpenSearch's
  `Describe*ReservedInstances`/`DescribeReservedDBInstances`/etc. calls are
  ordinary regional service APIs — they only return reservations that live
  in that one region, the same way `DescribeInstances` only lists instances
  in the region you call it against. A real reservation purchased in a
  different region than the data source's configured region won't appear,
  with no error (AWS correctly reports "none in this region"). If your
  reservations span multiple regions, either configure a separate data
  source per region or duplicate the relevant dashboard panel. Multi-region
  support in a single query is on the [Roadmap](../README.md#roadmap), not
  built yet.
- The bundled dashboard's **Reservations** and **Savings Plans** panels
  apply a "Filter data by values" transformation that excludes rows whose
  `State` is `retired`, so expired/returned reservations and Savings Plans
  don't clutter the default view. The backend itself returns every state
  AWS reports (`active`, `retired`, `payment-pending`, `payment-failed`,
  etc.) — edit or remove the panel's transformation if you want to see
  retired ones too.

## Reservation and Savings Plans coverage

Choose **Reservation coverage** or **Savings Plans coverage** as the
**Query type** to query AWS's `GetReservationCoverage` and
`GetSavingsPlansCoverage` APIs. Coverage answers a different question from
Utilization: Utilization asks "how much of what I bought am I using?",
Coverage asks "how much of my eligible usage is covered by what I bought,
versus paid at the On-Demand rate?". These two query types are time-range
scoped like Utilization (not snapshot queries like Inventory), use the same
Granularity/Result format/Linked account/Region controls, and the same
curated-metric-table-or-time-series presentation.

- **Reservation coverage's AWS service field is a required, fixed dropdown**,
  the same EC2-defaulting trap as Reservation utilization — but with a
  **narrower, four-value list** (EC2, RDS, ElastiCache, Redshift). AWS's own
  `GetReservationCoverage` documentation doesn't mention OpenSearch at all,
  unlike `GetReservationUtilization`'s five-service list — this is a real
  difference between the two APIs, not an oversight in this plugin's
  dropdown.
- Savings Plans coverage has no service filter, consistent with Savings
  Plans utilization.
- Neither coverage query type supports grouping in this plugin (including
  Reservation utilization's subscription-ID grouping) — a future
  enhancement, not built here.
- **Table** format shows a curated set of AWS's numeric fields: coverage %,
  On-Demand hours/cost not covered, reserved hours, and total running hours
  for Reservation coverage; coverage %, spend covered, On-Demand cost not
  covered, and total cost for Savings Plans coverage.
- **Time series** format plots one metric at a time, chosen via the
  **Utilization metric** selector (reused from Utilization — the field name
  is shared across all four query types, not specific to utilization).
- The bundled dashboard plots Coverage as a second line next to Utilization
  on the same chart (see [Example dashboard KPIs](#example-dashboard-kpis)
  below) rather than as separate summary stats — the gap between the two
  lines is the signal: utilization near 100% with coverage well below it
  means there's room to buy more and save further. AWS's own Coverage
  report additionally shows a "Potential monthly savings vs On-Demand"
  figure; that comes from a different API entirely
  (`GetSavingsPlansPurchaseRecommendation`/the equivalent Reservation
  recommendation APIs), which this plugin doesn't integrate with yet — see
  [Roadmap](../README.md#roadmap).
- AWS's coverage percentage fields are documented with an embedded `%`
  suffix (e.g. `"40%"`), unlike Utilization's plain-number percentage field —
  this plugin strips it before parsing, so the rendered value is a normal
  0-100 number like every other percent field.

## Example dashboard KPIs

The provisioned dashboard places three stat panels above the detailed charts:

- **Cost in selected period** sums ungrouped `UnblendedCost`.
- **Month-to-date cost** intentionally includes the incomplete current period
  and notes AWS reporting delay.
- **Change vs previous period** compares the selected range with the immediately
  preceding equivalent-duration range. Positive and negative changes are
  presented neutrally; a zero previous total returns no percentage.

Below those, a **Top 10 cost services** bar chart ranks services over the
selected range, and the AWS service time series splits tax onto its own
right-hand axis so it does not distort the per-service stacking. Two combined
trend charts, **Reservation utilization vs coverage % (RDS)** and **Savings
Plans utilization vs coverage %**, each plot two lines over the selected
range: Utilization (am I wasting what I bought?) and Coverage (could I be
buying more?) — see [Reservation and Savings Plans
coverage](#reservation-and-savings-plans-coverage) above. The Reservation
panel defaults to RDS — AWS only reports Reservation utilization/coverage one
service at a time; duplicate the panel and change both targets' AWS service
field to see EC2, ElastiCache, or Redshift instead. These show an error
rather than data if the optional
`ce:GetReservationUtilization`/`ce:GetSavingsPlansUtilization`/
`ce:GetReservationCoverage`/`ce:GetSavingsPlansCoverage` permissions aren't
granted, or no data if the account has no reservations/Savings Plans.
Finally, two table panels, **Reservations** and **Savings Plans**, list what
the account currently owns (see [Reservation and Savings Plans
inventory](#reservation-and-savings-plans-inventory) above) — unlike every
other panel on this dashboard, these ignore the selected time range.
