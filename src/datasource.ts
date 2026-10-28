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
    const queryType = query.queryType ?? 'costAndUsage';
    // The inventory query types carry no granularity/format/metric at all --
    // they're snapshot queries, not period-based -- so none of the checks
    // below apply. Hand-authored dashboard JSON (e.g. the bundled example
    // dashboard) for these never sets those fields, since they're
    // irrelevant; requiring them unconditionally silently dropped these
    // queries before Grafana ever sent them to the backend, the same way it
    // once did for the utilization query types below.
    if (queryType === 'reservationInventory' || queryType === 'savingsPlansInventory') {
      return true;
    }
    if (!query.granularity || !query.format) {
      return false;
    }
    // Only the default cost-and-usage query type needs a metric selected.
    // Hand-authored dashboard JSON (e.g. the bundled example dashboard) for
    // the utilization query types never sets one, since it's irrelevant
    // there -- requiring it unconditionally silently dropped those queries
    // before Grafana ever sent them to the backend.
    return queryType === 'costAndUsage' ? Boolean(query.metric) : true;
  }

  private async lookupValues(path: string, params: Record<string, string>): Promise<string[]> {
    try {
      const response = await this.getResource<{ values: string[] }>(path, params);
      return response.values ?? [];
    } catch (error) {
      console.warn(`AWS Cost Explorer: ${path} lookup failed`, error);
      return [];
    }
  }

  getDimensionValues(dimension: string, search: string): Promise<string[]> {
    return this.lookupValues('dimension-values', { dimension, search });
  }

  getCostAllocationTagKeys(search: string): Promise<string[]> {
    return this.lookupValues('tag-keys', { search });
  }

  getCostAllocationTagValues(key: string, search: string): Promise<string[]> {
    return this.lookupValues('tag-values', { key, search });
  }
}
