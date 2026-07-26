import { CoreApp, DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';
import { CostExplorerDataSourceOptions, CostQuery, DEFAULT_QUERY } from './types';

export class DataSource extends DataSourceWithBackend<CostQuery, CostExplorerDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<CostExplorerDataSourceOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<CostQuery> {
    return { ...DEFAULT_QUERY, filter: {}, groupBy: [] };
  }

  applyTemplateVariables(query: CostQuery, scopedVars: ScopedVars): CostQuery {
    const replace = (value?: string) => getTemplateSrv().replace(value ?? '', scopedVars);
    return {
      ...query,
      filter: {
        service: replace(query.filter.service),
        linkedAccount: replace(query.filter.linkedAccount),
        region: replace(query.filter.region),
        tagKey: replace(query.filter.tagKey),
        tagValue: replace(query.filter.tagValue),
      },
    };
  }

  filterQuery(query: CostQuery): boolean {
    return Boolean(query.metric && query.granularity && query.format);
  }
}
