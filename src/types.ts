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
export type QueryType = 'costAndUsage' | 'reservationUtilization' | 'savingsPlansUtilization';

// The only SERVICE filter values AWS accepts for GetReservationUtilization
// (one per request). AWS silently assumes the first value here if this
// filter is left unset, which can hide real reservations in another one of
// these services -- keep in sync with pkg/models/query.go's
// ReservationEligibleServices.
export type ReservationEligibleService =
  | 'Amazon Elastic Compute Cloud - Compute'
  | 'Amazon Relational Database Service'
  | 'Amazon ElastiCache'
  | 'Amazon Redshift'
  | 'Amazon Elasticsearch Service';

// Curated subsets of AWS's Reservation/Savings Plans utilization fields this
// plugin exposes as a selectable time-series metric or table column. Keep in
// sync with pkg/models/query.go's ReservationUtilizationMetrics /
// SavingsPlansUtilizationMetrics.
export type ReservationUtilizationMetric =
  | 'UtilizationPercentage'
  | 'NetRISavings'
  | 'OnDemandCostOfRIHoursUsed'
  | 'TotalAmortizedFee'
  | 'UnusedHours'
  | 'RealizedSavings'
  | 'UnrealizedSavings';
export type SavingsPlansUtilizationMetric =
  | 'UtilizationPercentage'
  | 'UsedCommitment'
  | 'UnusedCommitment'
  | 'TotalCommitment'
  | 'NetSavings'
  | 'OnDemandCostEquivalent'
  | 'TotalAmortizedCommitment';

export interface CostFilter {
  service?: string;
  linkedAccount?: string;
  region?: string;
  tagKey?: string;
  tagValue?: string;
}

export interface CostQuery extends DataQuery {
  version: 1;
  queryType: QueryType;
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
  groupBySubscription: boolean;
  utilizationMetric: ReservationUtilizationMetric | SavingsPlansUtilizationMetric;
}

export const DEFAULT_QUERY: CostQuery = {
  refId: 'A',
  version: 1,
  queryType: 'costAndUsage',
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
  groupBySubscription: false,
  utilizationMetric: 'UtilizationPercentage',
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
