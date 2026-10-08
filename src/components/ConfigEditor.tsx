import React, { ChangeEvent } from 'react';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { Alert, Combobox, InlineField, Input, SecretInput } from '@grafana/ui';
import { validateConfig } from '../configValidation';
import { AUTH_OPTIONS } from '../options';
import {
  AuthType,
  CostExplorerDataSourceOptions,
  CostExplorerSecureJsonData,
  normalizeDataSourceOptions,
} from '../types';

type Props = DataSourcePluginOptionsEditorProps<CostExplorerDataSourceOptions, CostExplorerSecureJsonData>;
type SecretKey = keyof CostExplorerSecureJsonData;

export function ConfigEditor({ onOptionsChange, options }: Props) {
  const jsonData = normalizeDataSourceOptions(options.jsonData);
  const errors = validateConfig(options);

  const updateJson = (patch: Partial<CostExplorerDataSourceOptions>) => {
    onOptionsChange({
      ...options,
      jsonData: { ...jsonData, ...patch },
    });
  };

  const updateSecret = (key: SecretKey, value: string) => {
    onOptionsChange(withSecureField(options, key, value));
  };

  const resetSecret = (key: SecretKey) => {
    onOptionsChange(withResetSecureField(options, key));
  };

  const secretInput = (key: SecretKey, id: string, placeholder: string) => (
    <SecretInput
      id={id}
      isConfigured={Boolean(options.secureJsonFields[key])}
      value={options.secureJsonData?.[key] ?? ''}
      placeholder={placeholder}
      width={48}
      onReset={() => resetSecret(key)}
      onChange={(event: ChangeEvent<HTMLInputElement>) => updateSecret(key, event.currentTarget.value)}
    />
  );

  return (
    <div>
      <h3>Authentication</h3>
      <InlineField
        label="Authentication Provider"
        labelWidth={24}
        htmlFor="config-auth-type"
        required
        tooltip="Specify which AWS credentials chain to use."
      >
        <Combobox<AuthType>
          id="config-auth-type"
          options={AUTH_OPTIONS}
          value={jsonData.authType}
          width={48}
          onChange={(value) => updateJson({ authType: value.value })}
        />
      </InlineField>
      <InlineField
        label="AWS region"
        labelWidth={24}
        required
        invalid={Boolean(errors.region)}
        error={errors.region}
        tooltip="Region used for the Cost Explorer endpoint and request signing."
      >
        <Input
          id="config-region"
          aria-label="AWS region"
          value={jsonData.region}
          placeholder="us-east-1"
          width={48}
          onChange={(event: ChangeEvent<HTMLInputElement>) => updateJson({ region: event.currentTarget.value })}
        />
      </InlineField>

      {jsonData.authType === 'default' && (
        <Alert title="No credentials are stored for this data source" severity="info">
          The AWS SDK default chain resolves the EC2 instance profile, ECS task role, EKS web identity, or the plugin
          process environment. Attach the Cost Explorer policy to the role the Grafana server already runs as. A Grafana
          administrator can withhold this by removing <code>default</code> from <code>allowed_auth_providers</code> in
          the <code>[aws]</code> configuration section.
        </Alert>
      )}

      {jsonData.authType === 'keys' && (
        <>
          <Alert title="Use short-lived credentials" severity="warning">
            Prefer temporary AWS credentials and rotate configured credentials regularly.
          </Alert>
          <InlineField label="Access key ID" labelWidth={24} required>
            {secretInput('accessKeyId', 'config-access-key-id', 'AWS access key ID')}
          </InlineField>
          <InlineField label="Secret access key" labelWidth={24} required>
            {secretInput('secretAccessKey', 'config-secret-access-key', 'AWS secret access key')}
          </InlineField>
          <InlineField label="Session token" labelWidth={24}>
            {secretInput('sessionToken', 'config-session-token', 'Optional temporary session token')}
          </InlineField>
          {errors.credentials && (
            <Alert title="Credentials are incomplete" severity="error">
              {errors.credentials}
            </Alert>
          )}
        </>
      )}

      <h3>Assume Role</h3>
      <InlineField
        label="Assume Role ARN"
        labelWidth={24}
        invalid={Boolean(errors.assumeRoleArn)}
        error={errors.assumeRoleArn}
        tooltip="Optional. Specifying the ARN of a role will ensure that the selected authentication provider is used to assume the role rather than the credentials directly."
      >
        <Input
          id="config-assume-role-arn"
          aria-label="Assume Role ARN"
          value={jsonData.assumeRoleArn ?? ''}
          placeholder="arn:aws:iam::123456789012:role/GrafanaCostExplorer"
          width={72}
          onChange={(event: ChangeEvent<HTMLInputElement>) => updateJson({ assumeRoleArn: event.currentTarget.value })}
        />
      </InlineField>
      {jsonData.assumeRoleArn && (
        <>
          <InlineField
            label="External ID"
            labelWidth={24}
            tooltip="Optional value required by some cross-account role trust policies. Stored as an encrypted secret."
          >
            {secretInput('externalId', 'config-external-id', 'Optional external ID')}
          </InlineField>
          <InlineField
            label="Role session name"
            labelWidth={24}
            invalid={Boolean(errors.roleSessionName)}
            error={errors.roleSessionName}
          >
            <Input
              id="config-role-session-name"
              aria-label="Role session name"
              value={jsonData.roleSessionName}
              width={48}
              onChange={(event: ChangeEvent<HTMLInputElement>) =>
                updateJson({ roleSessionName: event.currentTarget.value })
              }
            />
          </InlineField>
        </>
      )}

      <h3>Caching</h3>
      <InlineField
        label="TTL (seconds)"
        labelWidth={24}
        required
        invalid={Boolean(errors.cacheTTL)}
        error={errors.cacheTTL}
        tooltip="Identical Cost Explorer requests are cached in this plugin process."
      >
        <Input
          id="config-cache-ttl"
          aria-label="Cache TTL seconds"
          type="number"
          min={1}
          max={86400}
          value={jsonData.cacheTTLSeconds}
          width={24}
          onChange={(event: ChangeEvent<HTMLInputElement>) =>
            updateJson({ cacheTTLSeconds: Number(event.currentTarget.value) })
          }
        />
      </InlineField>
      <InlineField
        label="Maximum entries"
        labelWidth={24}
        required
        invalid={Boolean(errors.cacheMax)}
        error={errors.cacheMax}
      >
        <Input
          id="config-cache-max"
          aria-label="Cache maximum entries"
          type="number"
          min={1}
          max={10000}
          value={jsonData.cacheMaxEntries}
          width={24}
          onChange={(event: ChangeEvent<HTMLInputElement>) =>
            updateJson({ cacheMaxEntries: Number(event.currentTarget.value) })
          }
        />
      </InlineField>
    </div>
  );
}

export function withSecureField(options: Props['options'], key: SecretKey, value: string): Props['options'] {
  return {
    ...options,
    secureJsonData: {
      ...options.secureJsonData,
      [key]: value,
    },
  };
}

export function withResetSecureField(options: Props['options'], key: SecretKey): Props['options'] {
  return {
    ...options,
    secureJsonFields: {
      ...options.secureJsonFields,
      [key]: false,
    },
    secureJsonData: {
      ...options.secureJsonData,
      [key]: '',
    },
  };
}
