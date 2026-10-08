import { DataSourceSettings } from '@grafana/data';
import { CostExplorerDataSourceOptions, CostExplorerSecureJsonData, normalizeDataSourceOptions } from './types';

const roleArnPattern =
  /^arn:(aws|aws-us-gov|aws-cn|aws-iso|aws-iso-b):iam::[0-9]{12}:role\/[A-Za-z0-9+=,.@_/-]{1,512}$/;
const regionPattern = /^[a-z0-9][a-z0-9-]{0,62}$/;

export type ConfigErrors = Partial<
  Record<'region' | 'assumeRoleArn' | 'roleSessionName' | 'cacheTTL' | 'cacheMax' | 'credentials', string>
>;

export function validateConfig(
  options: Pick<
    DataSourceSettings<CostExplorerDataSourceOptions, CostExplorerSecureJsonData>,
    'jsonData' | 'secureJsonData' | 'secureJsonFields'
  >
): ConfigErrors {
  const settings = normalizeDataSourceOptions(options.jsonData);
  const errors: ConfigErrors = {};

  if (!regionPattern.test(settings.region) || !settings.region.includes('-')) {
    errors.region = 'Enter an AWS region such as us-east-1.';
  }
  if (settings.cacheTTLSeconds < 1 || settings.cacheTTLSeconds > 86400) {
    errors.cacheTTL = 'Cache TTL must be between 1 and 86400 seconds.';
  }
  if (settings.cacheMaxEntries < 1 || settings.cacheMaxEntries > 10000) {
    errors.cacheMax = 'Cache size must be between 1 and 10000 entries.';
  }

  if (settings.assumeRoleArn?.trim()) {
    if (!roleArnPattern.test(settings.assumeRoleArn)) {
      errors.assumeRoleArn = 'Enter a complete IAM role ARN.';
    }
    if (!/^[\w+=,.@-]{2,64}$/.test(settings.roleSessionName)) {
      errors.roleSessionName = 'Use 2-64 letters, numbers, or +=,.@- characters.';
    }
  }

  if (settings.authType === 'keys') {
    const hasAccessKey =
      Boolean(options.secureJsonFields.accessKeyId) || Boolean(options.secureJsonData?.accessKeyId?.trim());
    const hasSecret =
      Boolean(options.secureJsonFields.secretAccessKey) || Boolean(options.secureJsonData?.secretAccessKey?.trim());
    if (!hasAccessKey || !hasSecret) {
      errors.credentials = 'Access key ID and secret access key are required.';
    }
  }

  return errors;
}
