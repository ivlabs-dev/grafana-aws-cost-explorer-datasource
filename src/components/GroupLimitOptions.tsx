import React, { ChangeEvent } from 'react';
import { Combobox, InlineField, Switch } from '@grafana/ui';
import { TOP_N_OPTIONS } from '../options';
import { CostQuery, TopN } from '../types';

export interface GroupLimitOptionsProps {
  grouped: boolean;
  topN: TopN;
  includeOther: boolean;
  onChange: (change: Partial<Pick<CostQuery, 'topN' | 'includeOther'>>) => void;
}

export function GroupLimitOptions({ grouped, topN, includeOther, onChange }: GroupLimitOptionsProps) {
  if (!grouped) {
    return null;
  }

  return (
    <>
      <InlineField
        label="Limit groups"
        labelWidth={18}
        htmlFor="query-top-n"
        tooltip="Top groups are ranked by their total amount across the full query range."
      >
        <Combobox<TopN>
          id="query-top-n"
          options={TOP_N_OPTIONS}
          value={topN}
          width={24}
          onChange={(value) => onChange({ topN: value.value })}
        />
      </InlineField>
      {topN > 0 && (
        <InlineField
          label="Combine remaining groups as Other"
          labelWidth={32}
          htmlFor="query-include-other"
          tooltip="Groups outside the limit are summed into Other separately for each billing period and unit."
        >
          <Switch
            id="query-include-other"
            value={includeOther}
            onChange={(event: ChangeEvent<HTMLInputElement>) => onChange({ includeOther: event.currentTarget.checked })}
          />
        </InlineField>
      )}
    </>
  );
}
