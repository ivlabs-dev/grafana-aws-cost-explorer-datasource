import React from 'react';
import { InlineField, Switch } from '@grafana/ui';
import { CostQuery } from '../types';

interface Props {
  query: CostQuery;
  onChange: (patch: Partial<CostQuery>) => void;
}

export function PeriodOptions({ query, onChange }: Props) {
  return (
    <>
      <InlineField
        label="Include incomplete current period"
        labelWidth={30}
        htmlFor="query-include-incomplete-period"
        tooltip="AWS cost data for the current day or month may be incomplete."
      >
        <Switch
          id="query-include-incomplete-period"
          value={query.includeIncompletePeriod}
          onChange={(event) => onChange({ includeIncompletePeriod: event.currentTarget.checked })}
        />
      </InlineField>

      {query.granularity === 'MONTHLY' && (
        <InlineField
          label="Align to complete calendar months"
          labelWidth={30}
          htmlFor="query-align-monthly-to-calendar"
          tooltip="Uses UTC calendar-month boundaries. When disabled, the result may represent partial months."
        >
          <Switch
            id="query-align-monthly-to-calendar"
            value={query.alignMonthlyToCalendar}
            onChange={(event) => onChange({ alignMonthlyToCalendar: event.currentTarget.checked })}
          />
        </InlineField>
      )}
    </>
  );
}
