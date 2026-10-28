import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { QueryEditor } from './QueryEditor';
import { CostQuery, DEFAULT_QUERY } from '../types';

jest.mock('@grafana/ui', () => ({
  Alert: ({ title, children }: React.PropsWithChildren<{ title: string }>) => (
    <div>
      {title}
      {children}
    </div>
  ),
  InlineField: ({ children, htmlFor, label }: React.PropsWithChildren<{ htmlFor?: string; label?: string }>) => (
    <div>
      <label htmlFor={htmlFor}>{label}</label>
      {children}
    </div>
  ),
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => <input {...props} />,
  Switch: ({
    id,
    onChange,
    value,
  }: {
    id: string;
    onChange: React.ChangeEventHandler<HTMLInputElement>;
    value: boolean;
  }) => <input id={id} type="checkbox" checked={value} onChange={onChange} />,
  Combobox: ({
    'aria-label': ariaLabel,
    disabled,
    id,
    onChange,
    options,
    value,
  }: {
    'aria-label'?: string;
    disabled?: boolean;
    id: string;
    onChange: (value: { value: string } | null) => void;
    options:
      Array<{ label: string; value: string }> | ((input: string) => Promise<Array<{ label: string; value: string }>>);
    value: string;
  }) => {
    if (typeof options === 'function') {
      // Async comboboxes (the filter fields) are rendered as a plain input in
      // tests: the goal is asserting QueryEditor's wiring to the datasource
      // (that typing triggers a lookup, and that a typed value still commits
      // as a custom value), not re-testing Combobox's real async/select
      // behavior.
      return (
        <input
          id={id}
          aria-label={ariaLabel}
          disabled={disabled}
          value={value}
          onChange={(event) => {
            const typed = event.currentTarget.value;
            void options(typed);
            onChange(typed ? { value: typed } : null);
          }}
        />
      );
    }
    return (
      <select
        id={id}
        aria-label={ariaLabel}
        disabled={disabled}
        value={value}
        onChange={(event) => onChange({ value: event.currentTarget.value })}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    );
  },
}));

function createFakeDatasource() {
  return {
    getDimensionValues: jest.fn().mockResolvedValue([]),
    getCostAllocationTagKeys: jest.fn().mockResolvedValue([]),
    getCostAllocationTagValues: jest.fn().mockResolvedValue([]),
  };
}

function renderEditor(query: CostQuery = DEFAULT_QUERY, datasource = createFakeDatasource()) {
  const onChange = jest.fn();
  const onRunQuery = jest.fn();
  const view = render(
    <QueryEditor
      query={query}
      onChange={onChange}
      onRunQuery={onRunQuery}
      datasource={datasource as never}
      data={{} as never}
      range={{} as never}
    />
  );
  return { ...view, datasource, onChange, onRunQuery };
}

describe('QueryEditor', () => {
  it('normalizes the partial query Grafana supplies for a new panel', () => {
    renderEditor({ refId: 'A' } as CostQuery);
    expect(screen.getByLabelText('Metric')).toHaveValue('UnblendedCost');
    expect(screen.getByLabelText('Group by 2')).toBeDisabled();
    expect(screen.getByLabelText('AWS service filter')).toHaveValue('');
  });

  it('starts with production query defaults', () => {
    renderEditor();
    expect(screen.getByLabelText('Metric')).toHaveValue('UnblendedCost');
    expect(screen.getByLabelText('Granularity')).toHaveValue('DAILY');
    expect(screen.getByLabelText('Group by 2')).toBeDisabled();
    expect(screen.getByLabelText('Result format')).toHaveValue('timeSeries');
    expect(screen.getByLabelText('Include incomplete current period')).not.toBeChecked();
    expect(screen.queryByLabelText('Limit groups')).not.toBeInTheDocument();
  });

  it('changes metric and granularity', () => {
    const { onChange, onRunQuery } = renderEditor();
    fireEvent.change(screen.getByLabelText('Metric'), { target: { value: 'AmortizedCost' } });
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ metric: 'AmortizedCost' }));

    fireEvent.change(screen.getByLabelText('Granularity'), { target: { value: 'MONTHLY' } });
    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ granularity: 'MONTHLY' }));
    expect(onRunQuery).toHaveBeenCalledTimes(2);
  });

  it('adds two groupings and switches result format', () => {
    const first = renderEditor();
    fireEvent.change(screen.getByLabelText('Group by 1'), { target: { value: 'SERVICE' } });
    expect(first.onChange).toHaveBeenLastCalledWith(expect.objectContaining({ groupBy: ['SERVICE'] }));

    first.rerender(
      <QueryEditor
        query={{ ...DEFAULT_QUERY, groupBy: ['SERVICE'] }}
        onChange={first.onChange}
        onRunQuery={first.onRunQuery}
        datasource={first.datasource as never}
        data={{} as never}
        range={{} as never}
      />
    );
    fireEvent.change(screen.getByLabelText('Group by 2'), { target: { value: 'LINKED_ACCOUNT' } });
    expect(first.onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ groupBy: ['SERVICE', 'LINKED_ACCOUNT'] })
    );

    fireEvent.change(screen.getByLabelText('Result format'), { target: { value: 'table' } });
    expect(first.onChange).toHaveBeenLastCalledWith(expect.objectContaining({ format: 'table' }));
  });

  it('clears a hidden Top N value when grouping is removed', () => {
    const view = renderEditor({ ...DEFAULT_QUERY, groupBy: ['SERVICE'], topN: 5 });

    fireEvent.change(screen.getByLabelText('Group by 1'), { target: { value: '' } });

    expect(view.onChange).toHaveBeenLastCalledWith(expect.objectContaining({ groupBy: [], topN: 0 }));
  });

  it('shows group limits and monthly period controls only when relevant', () => {
    const view = renderEditor({
      ...DEFAULT_QUERY,
      granularity: 'MONTHLY',
      groupBy: ['SERVICE'],
      topN: 5,
    });

    expect(screen.getByLabelText('Limit groups')).toHaveValue('5');
    expect(screen.getByLabelText('Combine remaining groups as Other')).toBeChecked();
    expect(screen.getByLabelText('Align to complete calendar months')).toBeChecked();

    fireEvent.click(screen.getByLabelText('Include incomplete current period'));
    expect(view.onChange).toHaveBeenLastCalledWith(expect.objectContaining({ includeIncompletePeriod: true }));
  });

  it('looks up AWS service values as the user types and accepts a custom value', () => {
    const { datasource, onChange } = renderEditor();

    fireEvent.change(screen.getByLabelText('AWS service filter'), { target: { value: 'ec2' } });

    expect(datasource.getDimensionValues).toHaveBeenCalledWith('SERVICE', 'ec2');
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ filter: expect.objectContaining({ service: 'ec2' }) })
    );
  });

  it('looks up linked account and region values by their own dimension', () => {
    const { datasource } = renderEditor();

    fireEvent.change(screen.getByLabelText('Linked account filter'), { target: { value: '1234' } });
    expect(datasource.getDimensionValues).toHaveBeenCalledWith('LINKED_ACCOUNT', '1234');

    fireEvent.change(screen.getByLabelText('Region filter'), { target: { value: 'eu' } });
    expect(datasource.getDimensionValues).toHaveBeenCalledWith('REGION', 'eu');
  });

  it('disables the tag value field until a tag key is chosen, then scopes its lookup to that key', () => {
    const { datasource, rerender, onChange } = renderEditor();

    expect(screen.getByLabelText('Cost allocation tag value')).toBeDisabled();

    fireEvent.change(screen.getByLabelText('Cost allocation tag key'), { target: { value: 'Environment' } });
    expect(datasource.getCostAllocationTagKeys).toHaveBeenCalledWith('Environment');
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ filter: expect.objectContaining({ tagKey: 'Environment' }) })
    );

    rerender(
      <QueryEditor
        query={{ ...DEFAULT_QUERY, filter: { tagKey: 'Environment' } }}
        onChange={onChange}
        onRunQuery={jest.fn()}
        datasource={datasource as never}
        data={{} as never}
        range={{} as never}
      />
    );

    expect(screen.getByLabelText('Cost allocation tag value')).not.toBeDisabled();
    fireEvent.change(screen.getByLabelText('Cost allocation tag value'), { target: { value: 'prod' } });
    expect(datasource.getCostAllocationTagValues).toHaveBeenCalledWith('Environment', 'prod');
  });

  it('switches to Reservation utilization and shows its own fields, hiding cost-and-usage-only ones', () => {
    const first = renderEditor();

    fireEvent.change(screen.getByLabelText('Query type'), { target: { value: 'reservationUtilization' } });

    expect(first.onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({
        queryType: 'reservationUtilization',
        groupBy: [],
        topN: 0,
        // An explicit, visible service default -- matching AWS's own silent
        // EC2 default -- rather than an empty filter a user could mistake
        // for "all reservations".
        filter: { service: 'Amazon Elastic Compute Cloud - Compute' },
        groupBySubscription: false,
        utilizationMetric: 'UtilizationPercentage',
      })
    );

    first.rerender(
      <QueryEditor
        query={{ ...DEFAULT_QUERY, queryType: 'reservationUtilization' }}
        onChange={first.onChange}
        onRunQuery={first.onRunQuery}
        datasource={first.datasource as never}
        data={{} as never}
        range={{} as never}
      />
    );

    expect(screen.getByLabelText('AWS service')).toBeInTheDocument();
    expect(screen.getByLabelText('Group by subscription ID')).toBeInTheDocument();
    expect(screen.getByLabelText('Utilization metric')).toBeInTheDocument();
    expect(screen.queryByLabelText('Metric')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Group by 1')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Cost allocation tag key')).not.toBeInTheDocument();
  });

  it('constrains the Reservation utilization service filter to a fixed, required dropdown', () => {
    const view = renderEditor({
      ...DEFAULT_QUERY,
      queryType: 'reservationUtilization',
      filter: { service: 'Amazon Elastic Compute Cloud - Compute' },
    });

    const serviceField = screen.getByLabelText('AWS service') as HTMLSelectElement;
    expect(serviceField.tagName).toBe('SELECT');
    expect(serviceField).toHaveValue('Amazon Elastic Compute Cloud - Compute');
    const optionValues = Array.from(serviceField.options).map((option) => option.value);
    expect(optionValues).toEqual([
      'Amazon Elastic Compute Cloud - Compute',
      'Amazon Relational Database Service',
      'Amazon ElastiCache',
      'Amazon Redshift',
      'Amazon Elasticsearch Service',
    ]);

    fireEvent.change(serviceField, { target: { value: 'Amazon Relational Database Service' } });
    expect(view.onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ filter: expect.objectContaining({ service: 'Amazon Relational Database Service' }) })
    );
    expect(view.onRunQuery).toHaveBeenCalled();
  });

  it('hides the service filter and group-by-subscription switch for Savings Plans utilization', () => {
    renderEditor({ ...DEFAULT_QUERY, queryType: 'savingsPlansUtilization' });

    expect(screen.getByLabelText('Linked account filter')).toBeInTheDocument();
    expect(screen.getByLabelText('Region filter')).toBeInTheDocument();
    expect(screen.queryByLabelText('AWS service')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Group by subscription ID')).not.toBeInTheDocument();
  });

  it('only shows the utilization metric selector in time-series format', () => {
    const view = renderEditor({ ...DEFAULT_QUERY, queryType: 'reservationUtilization', format: 'table' });
    expect(screen.queryByLabelText('Utilization metric')).not.toBeInTheDocument();

    view.rerender(
      <QueryEditor
        query={{ ...DEFAULT_QUERY, queryType: 'reservationUtilization', format: 'timeSeries' }}
        onChange={view.onChange}
        onRunQuery={view.onRunQuery}
        datasource={view.datasource as never}
        data={{} as never}
        range={{} as never}
      />
    );
    expect(screen.getByLabelText('Utilization metric')).toBeInTheDocument();
  });

  it('toggles reservation utilization group-by-subscription', () => {
    const view = renderEditor({ ...DEFAULT_QUERY, queryType: 'reservationUtilization' });

    fireEvent.click(screen.getByLabelText('Group by subscription ID'));

    expect(view.onChange).toHaveBeenLastCalledWith(expect.objectContaining({ groupBySubscription: true }));
  });
});
