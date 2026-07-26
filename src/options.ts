import { ComboboxOption } from '@grafana/ui';
import { AuthMode, CostMetric, Granularity, GroupBy, ResultFormat } from './types';

export const AUTH_OPTIONS: Array<ComboboxOption<AuthMode>> = [
  { label: 'Default credential chain (recommended)', value: 'default' },
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
