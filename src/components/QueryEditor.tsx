import React, { ChangeEvent } from 'react';
import { QueryEditorProps } from '@grafana/data';
import { Alert, Combobox, ComboboxOption, InlineField, Switch } from '@grafana/ui';
import { DataSource } from '../datasource';
import {
  FORMAT_OPTIONS,
  GRANULARITY_OPTIONS,
  GROUP_OPTIONS,
  METRIC_OPTIONS,
  QUERY_TYPE_OPTIONS,
  RESERVATION_SERVICE_OPTIONS,
  RESERVATION_UTILIZATION_METRIC_OPTIONS,
  SAVINGS_PLANS_UTILIZATION_METRIC_OPTIONS,
} from '../options';
import { GroupLimitOptions } from './GroupLimitOptions';
import { PeriodOptions } from './PeriodOptions';
import {
  CostExplorerDataSourceOptions,
  CostMetric,
  CostQuery,
  Granularity,
  GroupBy,
  normalizeQuery,
  QueryType,
  ReservationEligibleService,
  ReservationUtilizationMetric,
  ResultFormat,
  SavingsPlansUtilizationMetric,
} from '../types';

type Props = QueryEditorProps<DataSource, CostQuery, CostExplorerDataSourceOptions>;

const toOptions = (values: string[]): Array<ComboboxOption<string>> => values.map((value) => ({ label: value, value }));

export function QueryEditor({ datasource, query, onChange, onRunQuery }: Props) {
  const model = normalizeQuery(query);

  const update = (patch: Partial<CostQuery>, run = true) => {
    onChange({ ...model, ...patch });
    if (run) {
      onRunQuery();
    }
  };

  const updateFilter = (key: keyof CostQuery['filter'], value: string) => {
    onChange({
      ...model,
      filter: { ...model.filter, [key]: value },
    });
  };

  const changeFirstGroup = (value: GroupBy | '') => {
    if (!value) {
      update({ groupBy: [], topN: 0 });
      return;
    }
    const second = model.groupBy[1] === value ? undefined : model.groupBy[1];
    update({ groupBy: second ? [value, second] : [value] });
  };

  const changeSecondGroup = (value: GroupBy | '') => {
    const first = model.groupBy[0];
    update({ groupBy: first && value && value !== first ? [first, value] : first ? [first] : [] });
  };

  const changeQueryType = (value: QueryType) => {
    // Switching query type resets fields that are specific to the previous
    // type, so a stale metric/groupBy/filter/utilization-metric can't
    // silently carry over into a query shape it doesn't apply to.
    // Reservation utilization gets an explicit, visible service default
    // (matching AWS's own silent EC2 default) rather than an empty filter --
    // leaving it empty doesn't mean "all reservations" to AWS.
    update({
      queryType: value,
      groupBy: [],
      topN: 0,
      filter: value === 'reservationUtilization' ? { service: 'Amazon Elastic Compute Cloud - Compute' } : {},
      groupBySubscription: false,
      utilizationMetric: 'UtilizationPercentage',
    });
  };

  const isReservationUtilization = model.queryType === 'reservationUtilization';
  const isSavingsPlansUtilization = model.queryType === 'savingsPlansUtilization';
  const isUtilization = isReservationUtilization || isSavingsPlansUtilization;
  const utilizationMetricOptions = isReservationUtilization
    ? RESERVATION_UTILIZATION_METRIC_OPTIONS
    : SAVINGS_PLANS_UTILIZATION_METRIC_OPTIONS;

  return (
    <div>
      <InlineField label="Query type" labelWidth={18} htmlFor="query-type" required>
        <Combobox<QueryType>
          id="query-type"
          options={QUERY_TYPE_OPTIONS}
          value={model.queryType}
          width={32}
          onChange={(value) => changeQueryType(value.value)}
        />
      </InlineField>

      {model.queryType === 'costAndUsage' && (
        <>
          <InlineField label="Metric" labelWidth={18} htmlFor="query-metric" required>
            <Combobox<CostMetric>
              id="query-metric"
              options={METRIC_OPTIONS}
              value={model.metric}
              width={32}
              onChange={(value) => update({ metric: value.value })}
            />
          </InlineField>
          <InlineField label="Granularity" labelWidth={18} htmlFor="query-granularity" required>
            <Combobox<Granularity>
              id="query-granularity"
              options={GRANULARITY_OPTIONS}
              value={model.granularity}
              width={24}
              onChange={(value) => update({ granularity: value.value })}
            />
          </InlineField>
          <InlineField label="Result format" labelWidth={18} htmlFor="query-format" required>
            <Combobox<ResultFormat>
              id="query-format"
              options={FORMAT_OPTIONS}
              value={model.format}
              width={24}
              onChange={(value) => update({ format: value.value })}
            />
          </InlineField>
          <InlineField label="Group by 1" labelWidth={18} htmlFor="query-group-1">
            <Combobox<GroupBy | ''>
              id="query-group-1"
              options={GROUP_OPTIONS}
              value={model.groupBy[0] ?? ''}
              width={32}
              onChange={(value) => changeFirstGroup(value.value)}
            />
          </InlineField>
          <InlineField label="Group by 2" labelWidth={18} htmlFor="query-group-2">
            <Combobox<GroupBy | ''>
              id="query-group-2"
              options={GROUP_OPTIONS.filter((option) => option.value !== model.groupBy[0])}
              value={model.groupBy[1] ?? ''}
              width={32}
              disabled={!model.groupBy[0]}
              onChange={(value) => changeSecondGroup(value.value)}
            />
          </InlineField>

          <InlineField
            label="AWS service"
            labelWidth={18}
            tooltip="Cost Explorer SERVICE dimension value. Type to search, or enter any value."
          >
            <Combobox<string>
              id="query-filter-service"
              aria-label="AWS service filter"
              options={(input) => datasource.getDimensionValues('SERVICE', input).then(toOptions)}
              value={model.filter.service ?? ''}
              placeholder="Amazon Elastic Compute Cloud - Compute"
              width={48}
              createCustomValue
              isClearable
              onChange={(option) => updateFilter('service', option?.value ?? '')}
              onBlur={onRunQuery}
            />
          </InlineField>
          <InlineField label="Linked account" labelWidth={18}>
            <Combobox<string>
              id="query-filter-account"
              aria-label="Linked account filter"
              options={(input) => datasource.getDimensionValues('LINKED_ACCOUNT', input).then(toOptions)}
              value={model.filter.linkedAccount ?? ''}
              placeholder="123456789012"
              width={32}
              createCustomValue
              isClearable
              onChange={(option) => updateFilter('linkedAccount', option?.value ?? '')}
              onBlur={onRunQuery}
            />
          </InlineField>
          <InlineField label="Region" labelWidth={18}>
            <Combobox<string>
              id="query-filter-region"
              aria-label="Region filter"
              options={(input) => datasource.getDimensionValues('REGION', input).then(toOptions)}
              value={model.filter.region ?? ''}
              placeholder="eu-west-1"
              width={32}
              createCustomValue
              isClearable
              onChange={(option) => updateFilter('region', option?.value ?? '')}
              onBlur={onRunQuery}
            />
          </InlineField>
          <InlineField label="Tag key" labelWidth={18}>
            <Combobox<string>
              id="query-filter-tag-key"
              aria-label="Cost allocation tag key"
              options={(input) => datasource.getCostAllocationTagKeys(input).then(toOptions)}
              value={model.filter.tagKey ?? ''}
              placeholder="Environment"
              width={32}
              createCustomValue
              isClearable
              onChange={(option) => updateFilter('tagKey', option?.value ?? '')}
              onBlur={onRunQuery}
            />
          </InlineField>
          <InlineField label="Tag value" labelWidth={18}>
            <Combobox<string>
              id="query-filter-tag-value"
              aria-label="Cost allocation tag value"
              options={(input) =>
                datasource.getCostAllocationTagValues(model.filter.tagKey ?? '', input).then(toOptions)
              }
              value={model.filter.tagValue ?? ''}
              placeholder="production"
              width={32}
              disabled={!model.filter.tagKey}
              createCustomValue
              isClearable
              onChange={(option) => updateFilter('tagValue', option?.value ?? '')}
              onBlur={onRunQuery}
            />
          </InlineField>

          <GroupLimitOptions
            grouped={model.groupBy.length > 0}
            topN={model.topN}
            includeOther={model.includeOther}
            onChange={(patch) => update(patch)}
          />
        </>
      )}

      {isUtilization && (
        <>
          <InlineField label="Granularity" labelWidth={18} htmlFor="query-granularity" required>
            <Combobox<Granularity>
              id="query-granularity"
              options={GRANULARITY_OPTIONS}
              value={model.granularity}
              width={24}
              onChange={(value) => update({ granularity: value.value })}
            />
          </InlineField>
          <InlineField label="Result format" labelWidth={18} htmlFor="query-format" required>
            <Combobox<ResultFormat>
              id="query-format"
              options={FORMAT_OPTIONS}
              value={model.format}
              width={24}
              onChange={(value) => update({ format: value.value })}
            />
          </InlineField>

          {isReservationUtilization && (
            <InlineField
              label="AWS service"
              labelWidth={18}
              htmlFor="query-filter-reservation-service"
              required
              tooltip="AWS only reports Reservation utilization one service at a time, and silently assumes EC2 if this isn't set -- which can make real reservations in another service (RDS being the most common case) look like no reservations at all."
            >
              <Combobox<ReservationEligibleService>
                id="query-filter-reservation-service"
                options={RESERVATION_SERVICE_OPTIONS}
                value={model.filter.service as ReservationEligibleService}
                width={48}
                onChange={(value) => {
                  updateFilter('service', value.value);
                  onRunQuery();
                }}
              />
            </InlineField>
          )}
          <InlineField label="Linked account" labelWidth={18}>
            <Combobox<string>
              id="query-filter-account"
              aria-label="Linked account filter"
              options={(input) => datasource.getDimensionValues('LINKED_ACCOUNT', input).then(toOptions)}
              value={model.filter.linkedAccount ?? ''}
              placeholder="123456789012"
              width={32}
              createCustomValue
              isClearable
              onChange={(option) => updateFilter('linkedAccount', option?.value ?? '')}
              onBlur={onRunQuery}
            />
          </InlineField>
          <InlineField label="Region" labelWidth={18}>
            <Combobox<string>
              id="query-filter-region"
              aria-label="Region filter"
              options={(input) => datasource.getDimensionValues('REGION', input).then(toOptions)}
              value={model.filter.region ?? ''}
              placeholder="eu-west-1"
              width={32}
              createCustomValue
              isClearable
              onChange={(option) => updateFilter('region', option?.value ?? '')}
              onBlur={onRunQuery}
            />
          </InlineField>

          {isReservationUtilization && (
            <InlineField
              label="Group by subscription ID"
              labelWidth={32}
              htmlFor="query-group-by-subscription"
              tooltip="AWS only supports grouping Reservation Utilization by subscription ID. When enabled, each subscription aggregates over the whole selected range rather than breaking it down further by period."
            >
              <Switch
                id="query-group-by-subscription"
                value={model.groupBySubscription}
                onChange={(event: ChangeEvent<HTMLInputElement>) =>
                  update({ groupBySubscription: event.currentTarget.checked })
                }
              />
            </InlineField>
          )}

          {model.format === 'timeSeries' && (
            <InlineField label="Utilization metric" labelWidth={18} htmlFor="query-utilization-metric" required>
              <Combobox<ReservationUtilizationMetric | SavingsPlansUtilizationMetric>
                id="query-utilization-metric"
                options={utilizationMetricOptions}
                value={model.utilizationMetric}
                width={40}
                onChange={(value) => update({ utilizationMetric: value.value })}
              />
            </InlineField>
          )}
        </>
      )}

      <PeriodOptions query={model} onChange={(patch) => update(patch)} />

      {model.queryType === 'costAndUsage' && model.metric === 'UsageQuantity' && (
        <Alert title="Usage quantities can have different units" severity="warning">
          Filter to a meaningful service or usage type before aggregating UsageQuantity.
        </Alert>
      )}
      {model.queryType === 'costAndUsage' && Boolean(model.filter.tagKey) !== Boolean(model.filter.tagValue) && (
        <Alert title="Complete the tag filter" severity="warning">
          Both tag key and tag value are required.
        </Alert>
      )}
    </div>
  );
}
