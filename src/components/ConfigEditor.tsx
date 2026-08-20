import React, { ChangeEvent } from 'react';
import { DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { Alert, Combobox, InlineField, Input, SecretInput } from '@grafana/ui';
import { validateConfig } from '../configValidation';
import { AUTH_OPTIONS } from '../options';
import {
  AuthMode,
  CostExplorerDataSourceOptions,
  CostExplorerSecureJsonData,
  DEFAULT_DATASOURCE_OPTIONS,
} from '../types';

type Props = DataSourcePluginOptionsEditorProps<CostExplorerDataSourceOptions, CostExplorerSecureJsonData>;
type SecretKey = keyof CostExplorerSecureJsonData;

export function ConfigEditor({ onOptionsChange, options }: Props) {
  const jsonData = { ...DEFAULT_DATASOURCE_OPTIONS, ...options.jsonData };
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
      <InlineField label="Authentication" labelWidth={24} htmlFor="config-auth-mode" required>
        <Combobox<AuthMode>
          id="config-auth-mode"
          options={AUTH_OPTIONS}
          value={jsonData.authMode}
          width={48}
          onChange={(value) => updateJson({ authMode: value.value })}
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

      {jsonData.authMode === 'assumeRole' && (
        <>
          <InlineField
            label="Role ARN"
            labelWidth={24}
            required
            invalid={Boolean(errors.roleArn)}
            error={errors.roleArn}
          >
            <Input
              id="config-role-arn"
              aria-label="Role ARN"
              value={options.jsonData.roleArn ?? ''}
              placeholder="arn:aws:iam::123456789012:role/GrafanaCostExplorer"
              width={72}
              onChange={(event: ChangeEvent<HTMLInputElement>) => updateJson({ roleArn: event.currentTarget.value })}
            />
          </InlineField>
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

      {(jsonData.authMode === 'static' || jsonData.authMode === 'assumeRole') && (
        <>
          {jsonData.authMode === 'static' ? (
            <Alert title="Use short-lived credentials" severity="warning">
              Prefer temporary AWS credentials and rotate configured credentials regularly.
            </Alert>
          ) : (
            <Alert title="AssumeRole source credentials" severity="info">
              These explicit credentials are used only to authenticate the STS AssumeRole request.
            </Alert>
          )}
          <InlineField
            label={jsonData.authMode === 'assumeRole' ? 'Source access key ID' : 'Access key ID'}
            labelWidth={24}
            required
          >
            {secretInput('accessKeyId', 'config-access-key-id', 'AWS access key ID')}
          </InlineField>
          <InlineField
            label={jsonData.authMode === 'assumeRole' ? 'Source secret access key' : 'Secret access key'}
            labelWidth={24}
            required
          >
            {secretInput('secretAccessKey', 'config-secret-access-key', 'AWS secret access key')}
          </InlineField>
          <InlineField
            label={jsonData.authMode === 'assumeRole' ? 'Source session token' : 'Session token'}
            labelWidth={24}
          >
            {secretInput('sessionToken', 'config-session-token', 'Optional temporary session token')}
          </InlineField>
          {errors.credentials && (
            <Alert title="Credentials are incomplete" severity="error">
              {errors.credentials}
            </Alert>
          )}
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
