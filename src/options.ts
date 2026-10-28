import { ComboboxOption } from '@grafana/ui';
import {
  AuthMode,
  CostMetric,
  Granularity,
  GroupBy,
  QueryType,
  ReservationEligibleService,
  ReservationUtilizationMetric,
  ResultFormat,
  SavingsPlansUtilizationMetric,
  TopN,
} from './types';

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

export const RESERVATION_SERVICE_OPTIONS: Array<ComboboxOption<ReservationEligibleService>> = [
  { label: 'EC2 (Amazon Elastic Compute Cloud - Compute)', value: 'Amazon Elastic Compute Cloud - Compute' },
  { label: 'RDS (Amazon Relational Database Service)', value: 'Amazon Relational Database Service' },
  { label: 'ElastiCache (Amazon ElastiCache)', value: 'Amazon ElastiCache' },
  { label: 'Redshift (Amazon Redshift)', value: 'Amazon Redshift' },
  { label: 'OpenSearch (Amazon Elasticsearch Service)', value: 'Amazon Elasticsearch Service' },
];

export const QUERY_TYPE_OPTIONS: Array<ComboboxOption<QueryType>> = [
  { label: 'Cost and usage', value: 'costAndUsage' },
  { label: 'Reservation utilization', value: 'reservationUtilization' },
  { label: 'Savings Plans utilization', value: 'savingsPlansUtilization' },
];

export const RESERVATION_UTILIZATION_METRIC_OPTIONS: Array<ComboboxOption<ReservationUtilizationMetric>> = [
  { label: 'Utilization %', value: 'UtilizationPercentage' },
  { label: 'Net RI savings', value: 'NetRISavings' },
  { label: 'On-Demand cost of RI hours used', value: 'OnDemandCostOfRIHoursUsed' },
  { label: 'Total amortized fee', value: 'TotalAmortizedFee' },
  { label: 'Unused hours', value: 'UnusedHours' },
  { label: 'Realized savings', value: 'RealizedSavings' },
  { label: 'Unrealized savings', value: 'UnrealizedSavings' },
];

export const SAVINGS_PLANS_UTILIZATION_METRIC_OPTIONS: Array<ComboboxOption<SavingsPlansUtilizationMetric>> = [
  { label: 'Utilization %', value: 'UtilizationPercentage' },
  { label: 'Used commitment', value: 'UsedCommitment' },
  { label: 'Unused commitment', value: 'UnusedCommitment' },
  { label: 'Total commitment', value: 'TotalCommitment' },
  { label: 'Net savings', value: 'NetSavings' },
  { label: 'On-Demand cost equivalent', value: 'OnDemandCostEquivalent' },
  { label: 'Total amortized commitment', value: 'TotalAmortizedCommitment' },
];
