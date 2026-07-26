import { expect, test } from '@grafana/plugin-e2e';

test('smoke: renders authentication and cache configuration', async ({
  createDataSourceConfigPage,
  readProvisionedDataSource,
  page,
}) => {
  const dataSource = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await createDataSourceConfigPage({ type: dataSource.type });
  await expect(page.getByRole('combobox', { name: 'Authentication' })).toBeVisible();
  await expect(page.getByRole('textbox', { name: 'AWS region' })).toHaveValue('us-east-1');
  await expect(page.getByRole('spinbutton', { name: 'Cache TTL seconds' })).toHaveValue('900');
});

test('optional AWS smoke: Save & test reaches Cost Explorer', async ({
  createDataSourceConfigPage,
  readProvisionedDataSource,
}) => {
  test.skip(process.env.AWS_INTEGRATION_TEST !== '1', 'Requires opt-in AWS credentials and ce:GetCostAndUsage');
  const dataSource = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  const configPage = await createDataSourceConfigPage({ type: dataSource.type });
  await expect(configPage.saveAndTest()).toBeOK();
});
