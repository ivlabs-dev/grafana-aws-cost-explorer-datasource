import { CostQuery, normalizeQuery } from './types';

describe('normalizeQuery', () => {
  it('applies compatible defaults to a legacy saved query', () => {
    const legacy = {
      refId: 'A',
      version: 1,
      metric: 'UnblendedCost',
      granularity: 'DAILY',
      groupBy: ['SERVICE'],
      filter: {},
      format: 'timeSeries',
    } as CostQuery;

    expect(normalizeQuery(legacy)).toEqual(
      expect.objectContaining({
        includeIncompletePeriod: false,
        topN: 0,
        includeOther: true,
        alignMonthlyToCalendar: true,
        rangeMode: 'dashboard',
      })
    );
  });

  it('preserves explicit false values', () => {
    const query = {
      refId: 'A',
      includeOther: false,
      alignMonthlyToCalendar: false,
    } as CostQuery;

    expect(normalizeQuery(query)).toEqual(
      expect.objectContaining({
        includeOther: false,
        alignMonthlyToCalendar: false,
      })
    );
  });
});
