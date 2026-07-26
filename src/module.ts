import { DataSourcePlugin } from '@grafana/data';
import { DataSource } from './datasource';
import { ConfigEditor } from './components/ConfigEditor';
import { QueryEditor } from './components/QueryEditor';
import { CostExplorerDataSourceOptions, CostQuery } from './types';

export const plugin = new DataSourcePlugin<DataSource, CostQuery, CostExplorerDataSourceOptions>(DataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
