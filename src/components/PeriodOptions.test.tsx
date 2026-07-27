import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { DEFAULT_QUERY } from '../types';
import { PeriodOptions } from './PeriodOptions';

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

describe('PeriodOptions', () => {
  it('shows the incomplete-period control with safe default and help text', () => {
    render(<PeriodOptions query={DEFAULT_QUERY} onChange={jest.fn()} />);

    expect(screen.getByLabelText('Include incomplete current period')).not.toBeChecked();
    expect(screen.getByText('AWS cost data for the current day or month may be incomplete.')).toBeInTheDocument();
    expect(screen.queryByLabelText('Align to complete calendar months')).not.toBeInTheDocument();
  });

  it('updates incomplete-period behavior', () => {
    const onChange = jest.fn();
    render(<PeriodOptions query={DEFAULT_QUERY} onChange={onChange} />);

    fireEvent.click(screen.getByLabelText('Include incomplete current period'));

    expect(onChange).toHaveBeenCalledWith({ includeIncompletePeriod: true });
  });

  it('shows and updates monthly alignment only for monthly queries', () => {
    const onChange = jest.fn();
    render(<PeriodOptions query={{ ...DEFAULT_QUERY, granularity: 'MONTHLY' }} onChange={onChange} />);

    expect(screen.getByLabelText('Align to complete calendar months')).toBeChecked();
    expect(
      screen.getByText('Uses UTC calendar-month boundaries. When disabled, the result may represent partial months.')
    ).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText('Align to complete calendar months'));

    expect(onChange).toHaveBeenCalledWith({ alignMonthlyToCalendar: false });
  });
});
