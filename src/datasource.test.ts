import { DataSource } from './datasource';
import { CostQuery, DEFAULT_QUERY } from './types';

// filterQuery doesn't reference `this`, so it can be called directly without
// constructing a full DataSourceWithBackend instance (which needs Grafana
// runtime globals this unit test environment doesn't provide).
const filterQuery = (query: CostQuery) => DataSource.prototype.filterQuery.call({} as DataSource, query);

describe('DataSource.filterQuery', () => {
  it('requires a metric for cost-and-usage queries', () => {
    expect(filterQuery(DEFAULT_QUERY)).toBe(true);
    expect(filterQuery({ ...DEFAULT_QUERY, metric: undefined as never })).toBe(false);
  });

  it('does not require a metric for the utilization query types', () => {
    // The bundled example dashboard's panels never set a metric for these
    // query types, since it's irrelevant there. Requiring one
    // unconditionally silently dropped those queries before Grafana ever
    // sent them to the backend -- this is the regression this test guards.
    const withoutMetric = { ...DEFAULT_QUERY, metric: undefined as never };
    expect(filterQuery({ ...withoutMetric, queryType: 'reservationUtilization' })).toBe(true);
    expect(filterQuery({ ...withoutMetric, queryType: 'savingsPlansUtilization' })).toBe(true);
  });

  it('still requires granularity and format regardless of query type', () => {
    expect(
      filterQuery({ ...DEFAULT_QUERY, queryType: 'reservationUtilization', granularity: undefined as never })
    ).toBe(false);
    expect(filterQuery({ ...DEFAULT_QUERY, queryType: 'reservationUtilization', format: undefined as never })).toBe(
      false
    );
  });

  it('treats a missing queryType (pre-existing saved queries) as cost-and-usage', () => {
    const { queryType: _queryType, ...legacyQuery } = DEFAULT_QUERY;
    expect(filterQuery(legacyQuery as CostQuery)).toBe(true);
    expect(filterQuery({ ...legacyQuery, metric: undefined as never } as unknown as CostQuery)).toBe(false);
  });
});
