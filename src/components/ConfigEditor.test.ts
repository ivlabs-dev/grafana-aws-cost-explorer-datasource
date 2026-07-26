import { DataSourceSettings } from '@grafana/data';
import { withResetSecureField, withSecureField } from './ConfigEditor';
import { CostExplorerDataSourceOptions, CostExplorerSecureJsonData } from '../types';

function options(): DataSourceSettings<CostExplorerDataSourceOptions, CostExplorerSecureJsonData> {
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
    jsonData: {},
    secureJsonFields: { secretAccessKey: true },
    secureJsonData: { accessKeyId: 'existing' },
    readOnly: false,
  };
}

describe('secure configuration helpers', () => {
  it('preserves other secure values when a field changes', () => {
    const changed = withSecureField(options(), 'sessionToken', 'temporary-token');
    expect(changed.secureJsonData).toEqual({
      accessKeyId: 'existing',
      sessionToken: 'temporary-token',
    });
    expect(changed.secureJsonFields.secretAccessKey).toBe(true);
  });

  it('marks only the reset field as unconfigured', () => {
    const reset = withResetSecureField(options(), 'secretAccessKey');
    expect(reset.secureJsonFields.secretAccessKey).toBe(false);
    expect(reset.secureJsonData?.secretAccessKey).toBe('');
    expect(reset.secureJsonData?.accessKeyId).toBe('existing');
  });
});
