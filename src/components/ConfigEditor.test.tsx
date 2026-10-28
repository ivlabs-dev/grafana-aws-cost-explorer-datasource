import React from 'react';
import { render, screen } from '@testing-library/react';
import { DataSourceSettings } from '@grafana/data';
import { ConfigEditor, withResetSecureField, withSecureField } from './ConfigEditor';
import { CostExplorerDataSourceOptions, CostExplorerSecureJsonData, DEFAULT_DATASOURCE_OPTIONS } from '../types';

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
  SecretInput: ({
    id,
    isConfigured,
    onChange,
    onReset,
    value,
  }: {
    id: string;
    isConfigured: boolean;
    value: string;
    onChange: React.ChangeEventHandler<HTMLInputElement>;
    onReset: () => void;
  }) =>
    isConfigured ? (
      <button type="button" onClick={onReset}>
        Reset
      </button>
    ) : (
      <input id={id} value={value} onChange={onChange} />
    ),
  Combobox: ({
    id,
    onChange,
    options,
    value,
  }: {
    id: string;
    onChange: (value: { value: string } | null) => void;
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

function baseOptions(): DataSourceSettings<CostExplorerDataSourceOptions, CostExplorerSecureJsonData> {
  return {
    id: 1,
    uid: 'test',
    orgId: 1,
    name: 'AWS Cost Explorer',
    type: 'ivlabsdev-awscostexplorer-datasource',
    typeName: 'AWS Cost Explorer',
    typeLogoUrl: '',
    access: 'proxy',
    url: '',
    user: '',
    database: '',
    basicAuth: false,
    basicAuthUser: '',
    withCredentials: false,
    isDefault: false,
    jsonData: { ...DEFAULT_DATASOURCE_OPTIONS },
    secureJsonFields: { secretAccessKey: true },
    secureJsonData: { accessKeyId: 'existing' },
    readOnly: false,
  };
}

describe('secure configuration helpers', () => {
  it('preserves other secure values when a field changes', () => {
    const changed = withSecureField(baseOptions(), 'sessionToken', 'temporary-token');
    expect(changed.secureJsonData).toEqual({
      accessKeyId: 'existing',
      sessionToken: 'temporary-token',
    });
    expect(changed.secureJsonFields.secretAccessKey).toBe(true);
  });

  it('marks only the reset field as unconfigured', () => {
    const reset = withResetSecureField(baseOptions(), 'secretAccessKey');
    expect(reset.secureJsonFields.secretAccessKey).toBe(false);
    expect(reset.secureJsonData?.secretAccessKey).toBe('');
    expect(reset.secureJsonData?.accessKeyId).toBe('existing');
  });
});

describe('ConfigEditor', () => {
  it('shows the AWS region field and no mock-mode alert for static credentials', () => {
    render(<ConfigEditor options={baseOptions()} onOptionsChange={jest.fn()} />);

    expect(screen.getByLabelText('AWS region')).toBeInTheDocument();
    expect(screen.queryByText(/No AWS account needed/)).not.toBeInTheDocument();
  });

  it('hides the AWS region and credential fields and shows the info alert for mock mode', () => {
    const options = baseOptions();
    options.jsonData = { ...options.jsonData, authMode: 'mock' };

    render(<ConfigEditor options={options} onOptionsChange={jest.fn()} />);

    expect(screen.getByText(/No AWS account needed/)).toBeInTheDocument();
    expect(screen.queryByLabelText('AWS region')).not.toBeInTheDocument();
    expect(screen.queryByText('Access key ID')).not.toBeInTheDocument();
    expect(screen.queryByText('Secret access key')).not.toBeInTheDocument();
  });
});
