import { ComboboxOption } from '@grafana/ui';
import { AuthMode, CostMetric, Granularity, GroupBy, ResultFormat, TopN } from './types';

export const AUTH_OPTIONS: Array<ComboboxOption<AuthMode>> = [
  { label: 'Assume an IAM role', value: 'assumeRole' },
  { label: 'Static credentials', value: 'static' },
];

export const METRIC_OPTIONS: Array<ComboboxOption<CostMetric>> = [
  { label: 'Unblended cost', value: 'UnblendedCost' },
  { label: 'Blended cost', value: 'BlendedCost' },
  { label: 'Amortized cost', value: 'AmortizedCost' },
  { label: 'Net amortized cost', value: 'NetAmortizedCost' },
  { label: 'Net unblended cost', value: 'NetUnblendedCost' },
  { label: 'Usage quantity', value: 'UsageQuantity' },
];

export const GRANULARITY_OPTIONS: Array<ComboboxOption<Granularity>> = [
  { label: 'Daily', value: 'DAILY' },
  { label: 'Monthly', value: 'MONTHLY' },
];

export const GROUP_OPTIONS: Array<ComboboxOption<GroupBy | ''>> = [
  { label: 'None', value: '' },
  { label: 'AWS service', value: 'SERVICE' },
  { label: 'Linked account', value: 'LINKED_ACCOUNT' },
  { label: 'Region', value: 'REGION' },
  { label: 'Instance type', value: 'INSTANCE_TYPE' },
  { label: 'Purchase type', value: 'PURCHASE_TYPE' },
  { label: 'Usage type', value: 'USAGE_TYPE' },
  { label: 'Operation', value: 'OPERATION' },
  { label: 'Availability zone', value: 'AVAILABILITY_ZONE' },
];

export const FORMAT_OPTIONS: Array<ComboboxOption<ResultFormat>> = [
  { label: 'Time series', value: 'timeSeries' },
  { label: 'Table', value: 'table' },
];

export const TOP_N_OPTIONS: Array<ComboboxOption<TopN>> = [
  { label: 'All', value: 0 },
  { label: 'Top 5', value: 5 },
  { label: 'Top 10', value: 10 },
  { label: 'Top 20', value: 20 },
];
