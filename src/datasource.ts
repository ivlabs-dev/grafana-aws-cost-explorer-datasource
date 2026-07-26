import { CoreApp, DataSourceInstanceSettings, ScopedVars } from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';
import { CostExplorerDataSourceOptions, CostQuery, DEFAULT_QUERY, normalizeQuery } from './types';

export class DataSource extends DataSourceWithBackend<CostQuery, CostExplorerDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<CostExplorerDataSourceOptions>) {
    super(instanceSettings);
  }

  getDefaultQuery(_: CoreApp): Partial<CostQuery> {
    return { ...DEFAULT_QUERY, filter: {}, groupBy: [] };
  }

  applyTemplateVariables(query: CostQuery, scopedVars: ScopedVars): CostQuery {
    const model = normalizeQuery(query);
    const replace = (value?: string) => getTemplateSrv().replace(value ?? '', scopedVars);
    return {
      ...model,
      filter: {
        service: replace(model.filter.service),
        linkedAccount: replace(model.filter.linkedAccount),
        region: replace(model.filter.region),
        tagKey: replace(model.filter.tagKey),
        tagValue: replace(model.filter.tagValue),
      },
    };
  }

  filterQuery(query: CostQuery): boolean {
    return Boolean(query.metric && query.granularity && query.format);
  }
}
