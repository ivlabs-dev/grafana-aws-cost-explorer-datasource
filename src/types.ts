import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type AuthMode = 'assumeRole' | 'static';
export type CostMetric =
  'UnblendedCost' | 'BlendedCost' | 'AmortizedCost' | 'NetAmortizedCost' | 'NetUnblendedCost' | 'UsageQuantity';
export type Granularity = 'DAILY' | 'MONTHLY';
export type GroupBy =
  | 'SERVICE'
  | 'LINKED_ACCOUNT'
  | 'REGION'
  | 'INSTANCE_TYPE'
  | 'PURCHASE_TYPE'
  | 'USAGE_TYPE'
  | 'OPERATION'
  | 'AVAILABILITY_ZONE';
export type ResultFormat = 'timeSeries' | 'table';
// Top 1 is reserved for provisioned/internal queries such as the highest-cost
// service KPI. The visual query editor intentionally offers only 0/5/10/20.
export type TopN = 0 | 1 | 5 | 10 | 20;
export type QueryRangeMode = 'dashboard' | 'monthToDate' | 'previousEquivalentPeriod';

export interface CostFilter {
  service?: string;
  linkedAccount?: string;
  region?: string;
  tagKey?: string;
  tagValue?: string;
}

export interface CostQuery extends DataQuery {
  version: 1;
  metric: CostMetric;
  granularity: Granularity;
  groupBy: GroupBy[];
  filter: CostFilter;
  format: ResultFormat;
  includeIncompletePeriod: boolean;
  topN: TopN;
  includeOther: boolean;
  alignMonthlyToCalendar: boolean;
  rangeMode: QueryRangeMode;
}

export const DEFAULT_QUERY: CostQuery = {
  refId: 'A',
  version: 1,
  metric: 'UnblendedCost',
  granularity: 'DAILY',
  groupBy: [],
  filter: {},
  format: 'timeSeries',
  includeIncompletePeriod: false,
  topN: 0,
  includeOther: true,
  alignMonthlyToCalendar: true,
  rangeMode: 'dashboard',
};

export function normalizeQuery(query: CostQuery): CostQuery {
  return {
    ...DEFAULT_QUERY,
    ...query,
    groupBy: query.groupBy ?? [],
    filter: query.filter ?? {},
  };
}

export interface CostExplorerDataSourceOptions extends DataSourceJsonData {
  authMode?: AuthMode;
  region?: string;
  roleArn?: string;
  roleSessionName?: string;
  cacheTTLSeconds?: number;
  cacheMaxEntries?: number;
}

export interface CostExplorerSecureJsonData {
  externalId?: string;
  accessKeyId?: string;
  secretAccessKey?: string;
  sessionToken?: string;
}

export const DEFAULT_DATASOURCE_OPTIONS: Required<
  Pick<CostExplorerDataSourceOptions, 'authMode' | 'region' | 'roleSessionName' | 'cacheTTLSeconds' | 'cacheMaxEntries'>
> = {
  authMode: 'static',
  region: 'us-east-1',
  roleSessionName: 'grafana-cost-explorer',
  cacheTTLSeconds: 900,
  cacheMaxEntries: 256,
};
