import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { GroupLimitOptions } from './GroupLimitOptions';

jest.mock('@grafana/ui', () => ({
  InlineField: ({
    children,
    htmlFor,
    label,
    tooltip,
  }: React.PropsWithChildren<{ htmlFor?: string; label?: string; tooltip?: string }>) => (
    <div>
      <label htmlFor={htmlFor}>{label}</label>
      <span>{tooltip}</span>
      {children}
    </div>
  ),
  Combobox: ({
    id,
    onChange,
    options,
    value,
  }: {
    id: string;
    onChange: (value: { label: string; value: number }) => void;
    options: Array<{ label: string; value: number }>;
    value: number;
  }) => (
    <select
      id={id}
      value={value}
      onChange={(event) => {
        const selected = options.find((option) => String(option.value) === event.currentTarget.value);
        if (selected) {
          onChange(selected);
        }
      }}
    >
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  ),
  Switch: ({
    id,
    onChange,
    value,
  }: {
    id: string;
    onChange: React.ChangeEventHandler<HTMLInputElement>;
    value: boolean;
  }) => <input id={id} type="checkbox" checked={value} onChange={onChange} />,
}));

describe('GroupLimitOptions', () => {
  it('is hidden when the query has no grouping', () => {
    render(<GroupLimitOptions grouped={false} topN={0} includeOther={true} onChange={jest.fn()} />);

    expect(screen.queryByLabelText('Limit groups')).not.toBeInTheDocument();
  });

  it('offers All, Top 5, Top 10, and Top 20 for grouped queries', () => {
    render(<GroupLimitOptions grouped topN={0} includeOther={true} onChange={jest.fn()} />);

    const options = screen.getAllByRole('option').map((option) => option.textContent);
    expect(options).toEqual(['All', 'Top 5', 'Top 10', 'Top 20']);
    expect(screen.getByText(/ranked by their total amount across the full query range/i)).toBeInTheDocument();
  });

  it('reports a Top N selection without changing the Other preference', () => {
    const onChange = jest.fn();
    render(<GroupLimitOptions grouped topN={0} includeOther={true} onChange={onChange} />);

    fireEvent.change(screen.getByLabelText('Limit groups'), { target: { value: '10' } });

    expect(onChange).toHaveBeenCalledWith({ topN: 10 });
  });

  it('shows and changes the Other option only while Top N is enabled', () => {
    const onChange = jest.fn();
    const view = render(<GroupLimitOptions grouped topN={5} includeOther={true} onChange={onChange} />);

    expect(screen.getByLabelText('Combine remaining groups as Other')).toBeChecked();
    expect(screen.getByText(/summed into Other separately for each billing period and unit/i)).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText('Combine remaining groups as Other'));
    expect(onChange).toHaveBeenCalledWith({ includeOther: false });

    view.rerender(<GroupLimitOptions grouped topN={0} includeOther={true} onChange={onChange} />);
    expect(screen.queryByLabelText('Combine remaining groups as Other')).not.toBeInTheDocument();
  });
});
