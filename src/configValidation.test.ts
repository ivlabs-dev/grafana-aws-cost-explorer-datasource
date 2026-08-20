import { validateConfig } from './configValidation';

describe('validateConfig', () => {
  it('accepts explicitly configured static credentials', () => {
    expect(
      validateConfig({
        jsonData: {
          authMode: 'static',
          region: 'us-east-1',
          cacheTTLSeconds: 900,
          cacheMaxEntries: 256,
        },
        secureJsonData: {
          accessKeyId: 'test-access-key',
          secretAccessKey: 'test-secret',
        },
        secureJsonFields: {},
      })
    ).toEqual({});
  });

  it('validates AssumeRole and static credential settings', () => {
    const assumeRole = validateConfig({
      jsonData: {
        authMode: 'assumeRole',
        region: 'us-east-1',
        roleArn: 'invalid',
        roleSessionName: 'x',
        cacheTTLSeconds: 900,
        cacheMaxEntries: 256,
      },
      secureJsonData: {},
      secureJsonFields: {},
    });
    expect(assumeRole.roleArn).toBeDefined();
    expect(assumeRole.roleSessionName).toBeDefined();
    expect(assumeRole.credentials).toBeDefined();

    const staticConfig = validateConfig({
      jsonData: {
        authMode: 'static',
        region: 'us-east-1',
        cacheTTLSeconds: 900,
        cacheMaxEntries: 256,
      },
      secureJsonData: {},
      secureJsonFields: {},
    });
    expect(staticConfig.credentials).toBeDefined();
  });

  it('recognizes already-configured secure fields', () => {
    expect(
      validateConfig({
        jsonData: {
          authMode: 'static',
          region: 'us-east-1',
          cacheTTLSeconds: 900,
          cacheMaxEntries: 256,
        },
        secureJsonData: {},
        secureJsonFields: { accessKeyId: true, secretAccessKey: true },
      }).credentials
    ).toBeUndefined();
  });

  it('requires explicit source credentials for AssumeRole', () => {
    const errors = validateConfig({
      jsonData: {
        authMode: 'assumeRole',
        region: 'us-east-1',
        roleArn: 'arn:aws:iam::123456789012:role/GrafanaCostExplorer',
        roleSessionName: 'grafana-cost-explorer',
        cacheTTLSeconds: 900,
        cacheMaxEntries: 256,
      },
      secureJsonData: {},
      secureJsonFields: {},
    });

    expect(errors.credentials).toBe('A source access key ID and secret access key are required.');
  });
});
