import React from 'react';
import { DataSourceSettings } from '@grafana/data';
import { render, screen } from '@testing-library/react';
import { ConfigEditor } from './ConfigEditor';
import { CostExplorerDataSourceOptions, CostExplorerSecureJsonData } from '../types';

jest.mock('@grafana/ui', () => ({
  Alert: ({ title, children }: React.PropsWithChildren<{ title: string }>) => (
    <div>
      {title}
      {children}
    </div>
  ),
  // Wraps the control so the label is associated even when the real component
  // would do it for us, which is how @grafana/ui behaves.
  InlineField: ({ children, htmlFor, label }: React.PropsWithChildren<{ htmlFor?: string; label?: string }>) => (
    <div>
      <label htmlFor={htmlFor}>
        {label}
        {children}
      </label>
    </div>
  ),
  Input: (props: React.InputHTMLAttributes<HTMLInputElement>) => <input {...props} />,
  SecretInput: ({ id, placeholder }: { id: string; placeholder: string }) => (
    <input id={id} placeholder={placeholder} />
  ),
  Combobox: ({
    id,
    onChange,
    options,
    value,
  }: {
    id: string;
    onChange: (value: { value: string }) => void;
    options: Array<{ label: string; value: string }>;
    value: string;
  }) => (
    <select id={id} value={value} onChange={(event) => onChange({ value: event.currentTarget.value })}>
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  ),
}));

function renderEditor(jsonData: CostExplorerDataSourceOptions) {
  const options = {
    jsonData,
    secureJsonData: {},
    secureJsonFields: {},
  } as unknown as DataSourceSettings<CostExplorerDataSourceOptions, CostExplorerSecureJsonData>;
  render(<ConfigEditor options={options} onOptionsChange={jest.fn()} />);
}

describe('ConfigEditor', () => {
  it('offers the AWS authentication providers by their Grafana names', () => {
    renderEditor({ authType: 'keys' });

    const provider = screen.getByLabelText('Authentication Provider') as HTMLSelectElement;
    expect(Array.from(provider.options).map((option) => option.text)).toEqual([
      'AWS SDK Default',
      'Access & secret key',
    ]);
    expect(provider.value).toBe('keys');
  });

  it('asks for an access key for the keys provider', () => {
    renderEditor({ authType: 'keys' });
    expect(screen.getByLabelText('Access key ID')).toBeInTheDocument();
    expect(screen.getByLabelText('Secret access key')).toBeInTheDocument();
  });

  it('asks for no credentials for the default provider', () => {
    renderEditor({ authType: 'default' });
    expect(screen.queryByLabelText('Access key ID')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Secret access key')).not.toBeInTheDocument();
  });

  it.each(['default', 'keys'] as const)('offers Assume Role ARN for the %s provider', (authType) => {
    renderEditor({ authType });
    expect(screen.getByLabelText('Assume Role ARN')).toBeInTheDocument();
  });

  it('hides the external ID and session name until a role is given', () => {
    renderEditor({ authType: 'default' });
    expect(screen.queryByLabelText('External ID')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Role session name')).not.toBeInTheDocument();
  });

  it('reveals the external ID and session name once a role is given', () => {
    renderEditor({ authType: 'default', assumeRoleArn: 'arn:aws:iam::123456789012:role/GrafanaCostExplorer' });
    expect(screen.getByLabelText('External ID')).toBeInTheDocument();
    expect(screen.getByLabelText('Role session name')).toBeInTheDocument();
  });

  it('shows a legacy assumeRole data source as keys plus a role', () => {
    renderEditor({ authMode: 'assumeRole', roleArn: 'arn:aws:iam::123456789012:role/GrafanaCostExplorer' });

    expect((screen.getByLabelText('Authentication Provider') as HTMLSelectElement).value).toBe('keys');
    expect(screen.getByLabelText('Assume Role ARN')).toHaveValue('arn:aws:iam::123456789012:role/GrafanaCostExplorer');
  });
});
