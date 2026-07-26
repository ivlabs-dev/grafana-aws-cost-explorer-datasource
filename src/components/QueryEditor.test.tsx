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
  Combobox: ({
    disabled,
    id,
    onChange,
    options,
    value,
  }: {
    disabled?: boolean;
    id: string;
    onChange: (value: { value: string }) => void;
    options: Array<{ label: string; value: string }>;
    value: string;
  }) => (
    <select
      id={id}
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
  ),
}));

function renderEditor(query: CostQuery = DEFAULT_QUERY) {
  const onChange = jest.fn();
  const onRunQuery = jest.fn();
  const view = render(
    <QueryEditor
      query={query}
      onChange={onChange}
      onRunQuery={onRunQuery}
      datasource={{} as never}
      data={{} as never}
      range={{} as never}
    />
  );
  return { ...view, onChange, onRunQuery };
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
        datasource={{} as never}
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
});
