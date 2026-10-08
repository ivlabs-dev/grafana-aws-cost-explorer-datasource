import { validateConfig } from './configValidation';
import { CostExplorerDataSourceOptions, CostExplorerSecureJsonData } from './types';

type Options = Parameters<typeof validateConfig>[0];

function config(
  jsonData: CostExplorerDataSourceOptions,
  secureJsonData: CostExplorerSecureJsonData = {},
  secureJsonFields: Record<string, boolean> = {}
): Options {
  return {
    jsonData: { region: 'us-east-1', cacheTTLSeconds: 900, cacheMaxEntries: 256, ...jsonData },
    secureJsonData,
    secureJsonFields,
  };
}

const accessKeys = { accessKeyId: 'test-access-key', secretAccessKey: 'test-secret' };
const roleArn = 'arn:aws:iam::123456789012:role/GrafanaCostExplorer';

describe('validateConfig', () => {
  it('accepts explicitly configured access keys', () => {
    expect(validateConfig(config({ authType: 'keys' }, accessKeys))).toEqual({});
  });

  it('validates AssumeRole and access key settings', () => {
    const assumeRole = validateConfig(config({ authType: 'keys', assumeRoleArn: 'invalid', roleSessionName: 'x' }));
    expect(assumeRole.assumeRoleArn).toBeDefined();
    expect(assumeRole.roleSessionName).toBeDefined();
    expect(assumeRole.credentials).toBeDefined();

    expect(validateConfig(config({ authType: 'keys' })).credentials).toBeDefined();
  });

  it('recognizes already-configured secure fields', () => {
    expect(
      validateConfig(config({ authType: 'keys' }, {}, { accessKeyId: true, secretAccessKey: true })).credentials
    ).toBeUndefined();
  });

  it('requires explicit source credentials for AssumeRole', () => {
    const errors = validateConfig(config({ authType: 'keys', assumeRoleArn: roleArn }));
    expect(errors.credentials).toBe('Access key ID and secret access key are required.');
  });

  it('requires no stored credentials for the default provider', () => {
    expect(validateConfig(config({ authType: 'default' }))).toEqual({});
  });

  it('still validates non-credential settings for the default provider', () => {
    expect(validateConfig(config({ authType: 'default', region: 'not a region' }))).toEqual({
      region: 'Enter an AWS region such as us-east-1.',
    });
  });

  describe.each(['default', 'keys'] as const)('with the %s provider', (authType) => {
    const secrets = authType === 'keys' ? accessKeys : {};

    it('accepts a complete role ARN', () => {
      expect(validateConfig(config({ authType, assumeRoleArn: roleArn }, secrets))).toEqual({});
    });

    it('ignores role settings when no ARN is given', () => {
      expect(validateConfig(config({ authType, roleSessionName: '!' }, secrets))).toEqual({});
    });
  });

  describe('legacy settings', () => {
    it('treats a legacy static data source as the keys provider', () => {
      expect(validateConfig(config({ authMode: 'static' }, accessKeys))).toEqual({});
      expect(validateConfig(config({ authMode: 'static' })).credentials).toBeDefined();
    });

    it('carries a legacy assumeRole ARN across', () => {
      expect(validateConfig(config({ authMode: 'assumeRole', roleArn }, accessKeys))).toEqual({});
      expect(
        validateConfig(config({ authMode: 'assumeRole', roleArn: 'invalid' }, accessKeys)).assumeRoleArn
      ).toBeDefined();
    });

    it('does not reinterpret an unrecognised mode as a provider of the same name', () => {
      expect(validateConfig(config({ authMode: 'default' })).credentials).toBeDefined();
    });
  });
});
