package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/awsclient"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/costexplorer"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/frames"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

const awsRequestTimeout = 30 * time.Second

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

type Datasource struct {
	settings           models.PluginSettings
	service            *costexplorer.Service
	resolveCredentials func(context.Context) error
	initializationErr  error
	logger             log.Logger
}

func NewDatasource(ctx context.Context, source backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	return newDatasource(ctx, source, awsclient.NewFactory(), log.DefaultLogger), nil
}

func newDatasource(
	ctx context.Context,
	source backend.DataSourceInstanceSettings,
	factory awsclient.Factory,
	logger log.Logger,
) *Datasource {
	datasource := &Datasource{logger: logger.With("datasource_uid", source.UID)}

	settings, err := models.LoadPluginSettings(source)
	if err != nil {
		datasource.initializationErr = err
		return datasource
	}
	datasource.settings = *settings
	if err := settings.Validate(); err != nil {
		datasource.initializationErr = err
		return datasource
	}

	resultCache, err := cache.NewMemory(
		time.Duration(settings.CacheTTLSeconds)*time.Second,
		settings.CacheMaxEntries,
	)
	if err != nil {
		datasource.initializationErr = err
		return datasource
	}

	bundle, err := factory.New(ctx, *settings)
	if err != nil {
		datasource.initializationErr = err
		return datasource
	}
	service, err := costexplorer.NewService(bundle.CostExplorer, resultCache)
	if err != nil {
		datasource.initializationErr = err
		return datasource
	}

	datasource.service = service
	datasource.resolveCredentials = bundle.ResolveCredentials
	return datasource
}

func (d *Datasource) Dispose() {}

func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()
	for _, query := range req.Queries {
		response.Responses[query.RefID] = d.query(ctx, query)
	}
	return response, nil
}

func (d *Datasource) query(ctx context.Context, query backend.DataQuery) backend.DataResponse {
	started := time.Now()
	logger := d.logger.FromContext(ctx).With("ref_id", query.RefID)

	if d.initializationErr != nil {
		logger.Error("query initialization failed", "error", d.initializationErr)
		return backend.ErrDataResponse(backend.StatusValidationFailed, d.initializationErr.Error())
	}

	var model models.Query
	if err := json.Unmarshal(query.JSON, &model); err != nil {
		logger.Warn("query model decoding failed", "error", err)
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("decode query model: %v", err))
	}
	model.ApplyDefaults()
	if err := model.Validate(); err != nil {
		logger.Warn("query validation failed", "error", err)
		return backend.ErrDataResponse(backend.StatusValidationFailed, err.Error())
	}

	dateRange, err := models.CostExplorerDateRange(query.TimeRange.From, query.TimeRange.To)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusValidationFailed, err.Error())
	}

	awsContext, cancel := context.WithTimeout(ctx, awsRequestTimeout)
	defer cancel()

	execution, err := d.service.Execute(awsContext, d.settings, model, dateRange)
	if err != nil {
		failedAt := time.Now()
		metadata := NewQueryExecutionMetadata(
			execution,
			failedAt,
			failedAt.Sub(started),
			model.IncludeIncompletePeriod,
		)
		fields := append(metadata.LogFields(), "error_category", "aws_cost_explorer_request_failed")
		logger.Error("Cost Explorer query failed", fields...)
		status := backend.StatusBadGateway
		if awsContext.Err() == context.DeadlineExceeded {
			status = backend.StatusTimeout
		}
		return backend.ErrDataResponse(status, err.Error())
	}

	resultFrames, err := frames.Convert(model, execution.Output)
	if err != nil {
		logger.Error("data frame conversion failed", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("convert AWS response to data frames: %v", err))
	}
	for _, frame := range resultFrames {
		frame.RefID = query.RefID
	}

	completedAt := time.Now()
	metadata := NewQueryExecutionMetadata(
		execution,
		completedAt,
		completedAt.Sub(started),
		model.IncludeIncompletePeriod,
	)
	AttachQueryExecutionMetadata(resultFrames, metadata)

	fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
	logger.Info("Cost Explorer query completed", fields...)
	logger.Debug(
		"Cost Explorer cache configuration",
		"maximum_entries", d.settings.CacheMaxEntries,
		"ttl_seconds", d.settings.CacheTTLSeconds,
	)

	return backend.DataResponse{
		Status: backend.StatusOK,
		Frames: resultFrames,
	}
}

func (d *Datasource) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if d.initializationErr != nil {
		return healthError(d.initializationErr), nil
	}
	if d.resolveCredentials == nil || d.service == nil {
		return healthError(fmt.Errorf("data source was not initialized")), nil
	}

	awsContext, cancel := context.WithTimeout(ctx, awsRequestTimeout)
	defer cancel()

	if err := d.resolveCredentials(awsContext); err != nil {
		classified := fmt.Errorf("AWS credentials could not be resolved; verify the selected credential source: %w", err)
		d.logger.Warn("AWS credential resolution failed", "error", classified)
		return healthError(classified), nil
	}

	now := time.Now().UTC()
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	from := to.AddDate(0, 0, -1)
	dateRange, err := models.CostExplorerDateRange(from, to)
	if err != nil {
		return healthError(err), nil
	}
	query := models.Query{
		Version:     models.QueryVersion,
		Metric:      "UnblendedCost",
		Granularity: models.GranularityDaily,
		Format:      models.FormatTable,
	}

	execution, err := d.service.Execute(awsContext, d.settings, query, dateRange)
	if err != nil {
		d.logger.Warn("Cost Explorer health check failed", "error", err)
		return healthError(err), nil
	}

	d.logger.Info(
		"Cost Explorer health check completed",
		"region", d.settings.Region,
		"auth_mode", d.settings.AuthMode,
		"cache", execution.CacheStatus(),
		"aws_duration_ms", execution.AWSDuration.Milliseconds(),
	)
	return &backend.CheckHealthResult{
		Status: backend.HealthStatusOk,
		Message: fmt.Sprintf(
			"Connected to AWS Cost Explorer in %s using %s credentials (cache %s)",
			d.settings.Region,
			d.settings.AuthMode,
			execution.CacheStatus(),
		),
	}, nil
}

func healthError(err error) *backend.CheckHealthResult {
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusError,
		Message: err.Error(),
	}
}
