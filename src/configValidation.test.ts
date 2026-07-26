import { validateConfig } from './configValidation';

describe('validateConfig', () => {
  it('accepts the default credential chain configuration', () => {
    expect(
      validateConfig({
        jsonData: {
          authMode: 'default',
          region: 'us-east-1',
          cacheTTLSeconds: 900,
          cacheMaxEntries: 256,
        },
        secureJsonData: {},
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
    expect(staticConfig.static).toBeDefined();
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
      }).static
    ).toBeUndefined();
  });
});
