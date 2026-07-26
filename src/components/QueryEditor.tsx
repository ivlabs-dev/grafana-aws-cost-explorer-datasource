import React, { ChangeEvent } from 'react';
import { QueryEditorProps } from '@grafana/data';
import { Alert, Combobox, InlineField, Input } from '@grafana/ui';
import { DataSource } from '../datasource';
import { FORMAT_OPTIONS, GRANULARITY_OPTIONS, GROUP_OPTIONS, METRIC_OPTIONS } from '../options';
import {
  CostExplorerDataSourceOptions,
  CostMetric,
  CostQuery,
  Granularity,
  GroupBy,
  normalizeQuery,
  ResultFormat,
} from '../types';

type Props = QueryEditorProps<DataSource, CostQuery, CostExplorerDataSourceOptions>;

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
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
      update({ groupBy: [] });
      return;
    }
    const second = model.groupBy[1] === value ? undefined : model.groupBy[1];
    update({ groupBy: second ? [value, second] : [value] });
  };

  const changeSecondGroup = (value: GroupBy | '') => {
    const first = model.groupBy[0];
    update({ groupBy: first && value && value !== first ? [first, value] : first ? [first] : [] });
  };

  return (
    <div>
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

      <InlineField label="AWS service" labelWidth={18} tooltip="Exact Cost Explorer SERVICE dimension value.">
        <Input
          id="query-filter-service"
          aria-label="AWS service filter"
          value={model.filter.service ?? ''}
          placeholder="Amazon Elastic Compute Cloud - Compute"
          width={48}
          onChange={(event: ChangeEvent<HTMLInputElement>) => updateFilter('service', event.currentTarget.value)}
          onBlur={onRunQuery}
        />
      </InlineField>
      <InlineField label="Linked account" labelWidth={18}>
        <Input
          id="query-filter-account"
          aria-label="Linked account filter"
          value={model.filter.linkedAccount ?? ''}
          placeholder="123456789012"
          width={32}
          onChange={(event: ChangeEvent<HTMLInputElement>) => updateFilter('linkedAccount', event.currentTarget.value)}
          onBlur={onRunQuery}
        />
      </InlineField>
      <InlineField label="Region" labelWidth={18}>
        <Input
          id="query-filter-region"
          aria-label="Region filter"
          value={model.filter.region ?? ''}
          placeholder="eu-west-1"
          width={32}
          onChange={(event: ChangeEvent<HTMLInputElement>) => updateFilter('region', event.currentTarget.value)}
          onBlur={onRunQuery}
        />
      </InlineField>
      <InlineField label="Tag key" labelWidth={18}>
        <Input
          id="query-filter-tag-key"
          aria-label="Cost allocation tag key"
          value={model.filter.tagKey ?? ''}
          placeholder="Environment"
          width={32}
          onChange={(event: ChangeEvent<HTMLInputElement>) => updateFilter('tagKey', event.currentTarget.value)}
          onBlur={onRunQuery}
        />
      </InlineField>
      <InlineField label="Tag value" labelWidth={18}>
        <Input
          id="query-filter-tag-value"
          aria-label="Cost allocation tag value"
          value={model.filter.tagValue ?? ''}
          placeholder="production"
          width={32}
          onChange={(event: ChangeEvent<HTMLInputElement>) => updateFilter('tagValue', event.currentTarget.value)}
          onBlur={onRunQuery}
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

      {model.metric === 'UsageQuantity' && (
        <Alert title="Usage quantities can have different units" severity="warning">
          Filter to a meaningful service or usage type before aggregating UsageQuantity.
        </Alert>
      )}
      {Boolean(model.filter.tagKey) !== Boolean(model.filter.tagValue) && (
        <Alert title="Complete the tag filter" severity="warning">
          Both tag key and tag value are required.
        </Alert>
      )}
    </div>
  );
}
