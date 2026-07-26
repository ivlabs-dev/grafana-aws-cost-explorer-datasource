import { expect, test } from '@grafana/plugin-e2e';

test('smoke: renders the visual Cost Explorer query editor', async ({
  panelEditPage,
  readProvisionedDataSource,
  page,
}) => {
  const dataSource = await readProvisionedDataSource({ fileName: 'datasources.yml' });
  await panelEditPage.datasource.set(dataSource.name);
  const row = panelEditPage.getQueryEditorRow('A');
  await expect(row.getByRole('combobox', { name: 'Metric' })).toBeVisible();
  await expect(row.getByRole('combobox', { name: 'Granularity' })).toBeVisible();
  await expect(row.getByRole('combobox', { name: 'Group by 1' })).toBeVisible();
  await expect(row.getByRole('combobox', { name: 'Result format' })).toBeVisible();
  await expect(page.getByRole('textbox', { name: 'AWS service filter' })).toBeVisible();
});
