import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

export type AuthMode = 'default' | 'assumeRole' | 'static';
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
}

export const DEFAULT_QUERY: CostQuery = {
  refId: 'A',
  version: 1,
  metric: 'UnblendedCost',
  granularity: 'DAILY',
  groupBy: [],
  filter: {},
  format: 'timeSeries',
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
  authMode: 'default',
  region: 'us-east-1',
  roleSessionName: 'grafana-cost-explorer',
  cacheTTLSeconds: 900,
  cacheMaxEntries: 256,
};
