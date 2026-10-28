package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/awsclient"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/costexplorer"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/frames"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/inventory"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

const (
	awsRequestTimeout                = 30 * time.Second
	credentialHealthFailureMessage   = "AWS credentials could not be validated. Check the configured credentials."
	costExplorerHealthFailureMessage = "AWS Cost Explorer request failed. Check the configured credentials, IAM permissions, and AWS region."
	// lookupWindowDays bounds how far back dimension-value and tag lookups
	// search. These values change rarely, so a fixed trailing window avoids
	// depending on any particular panel's dashboard time range.
	lookupWindowDays = 90
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

type Datasource struct {
	settings           models.PluginSettings
	service            *costexplorer.Service
	inventoryService   *inventory.Service
	resolveCredentials func(context.Context) error
	initializationErr  error
	logger             log.Logger
	now                func() time.Time
	resourceHandler    backend.CallResourceHandler
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
	datasource := &Datasource{
		logger: logger.With("datasource_uid", source.UID),
		now:    time.Now,
	}
	datasource.resourceHandler = datasource.buildResourceHandler()

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
	inventoryService, err := inventory.NewService(
		bundle.SavingsPlans,
		bundle.EC2Reservations,
		bundle.RDSReservations,
		bundle.ElastiCacheReservations,
		bundle.RedshiftReservations,
		bundle.OpenSearchReservations,
		resultCache,
	)
	if err != nil {
		datasource.initializationErr = err
		return datasource
	}

	datasource.service = service
	datasource.inventoryService = inventoryService
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
	started := d.now()
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

	// Inventory queries report current state ("what do I own right now"),
	// not a historical bucket, so they skip date-range resolution entirely
	// rather than just ignoring the result of it.
	switch model.QueryType {
	case models.QueryTypeReservationInventory:
		return d.queryReservationInventory(ctx, query, started, logger)
	case models.QueryTypeSavingsPlansInventory:
		return d.querySavingsPlansInventory(ctx, query, started, logger)
	}

	resolved, err := models.ResolveCostExplorerDateRange(
		model,
		query.TimeRange.From,
		query.TimeRange.To,
		started,
	)
	if err != nil {
		return backend.ErrDataResponse(backend.StatusValidationFailed, err.Error())
	}

	switch model.QueryType {
	case models.QueryTypeReservationUtilization:
		return d.queryReservationUtilization(ctx, query, model, resolved, started, logger)
	case models.QueryTypeSavingsPlansUtilization:
		return d.querySavingsPlansUtilization(ctx, query, model, resolved, started, logger)
	case models.QueryTypeReservationCoverage:
		return d.queryReservationCoverage(ctx, query, model, resolved, started, logger)
	case models.QueryTypeSavingsPlansCoverage:
		return d.querySavingsPlansCoverage(ctx, query, model, resolved, started, logger)
	default:
		return d.queryCostAndUsage(ctx, query, model, resolved, started, logger)
	}
}

func (d *Datasource) queryCostAndUsage(
	ctx context.Context,
	query backend.DataQuery,
	model models.Query,
	resolved models.ResolvedDateRange,
	started time.Time,
	logger log.Logger,
) backend.DataResponse {
	if resolved.Empty {
		resultFrames, convertErr := frames.Convert(model, &awscostexplorer.GetCostAndUsageOutput{})
		if convertErr != nil {
			return backend.ErrDataResponse(
				backend.StatusInternal,
				fmt.Sprintf("create empty AWS Cost Explorer data frames: %v", convertErr),
			)
		}
		for _, frame := range resultFrames {
			frame.RefID = query.RefID
			if frame.Meta == nil {
				frame.Meta = &data.FrameMeta{}
			}
			frame.Meta.Notices = append(frame.Meta.Notices, data.Notice{
				Severity: data.NoticeSeverityInfo,
				Text:     models.NoCompletedPeriodsNotice,
			})
		}

		completedAt := d.now()
		execution := &costexplorer.Execution{
			CacheResult: cache.ResultBypass,
			CacheTTL:    time.Duration(d.settings.CacheTTLSeconds) * time.Second,
		}
		metadata := NewQueryExecutionMetadata(
			execution,
			completedAt,
			completedAt.Sub(started),
			false,
		)
		AttachQueryExecutionMetadata(resultFrames, metadata)
		fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
		logger.Info("Cost Explorer query completed without an AWS request", fields...)
		return backend.DataResponse{Status: backend.StatusOK, Frames: resultFrames}
	}

	awsContext, cancel := context.WithTimeout(ctx, awsRequestTimeout)
	defer cancel()

	execution, err := d.service.Execute(awsContext, d.settings, model, resolved.DateRange)
	if err != nil {
		failedAt := d.now()
		metadata := NewQueryExecutionMetadata(
			execution,
			failedAt,
			failedAt.Sub(started),
			resolved.IncludesIncompletePeriod,
		)
		fields := append(metadata.LogFields(), "error_category", "aws_cost_explorer_request_failed")
		logger.Error("Cost Explorer query failed", fields...)
		status := backend.StatusBadGateway
		if awsContext.Err() == context.DeadlineExceeded {
			status = backend.StatusTimeout
		}
		errorResponse := backend.ErrDataResponse(status, err.Error())
		errorFrames, frameErr := frames.Convert(model, &awscostexplorer.GetCostAndUsageOutput{})
		if frameErr == nil {
			for _, frame := range errorFrames {
				frame.RefID = query.RefID
			}
			AttachQueryExecutionMetadata(errorFrames, metadata)
			errorResponse.Frames = errorFrames
		}
		return errorResponse
	}

	resultFrames, err := frames.Convert(model, execution.Output)
	if err != nil {
		logger.Error("data frame conversion failed", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("convert AWS response to data frames: %v", err))
	}
	for _, frame := range resultFrames {
		frame.RefID = query.RefID
	}

	completedAt := d.now()
	metadata := NewQueryExecutionMetadata(
		execution,
		completedAt,
		completedAt.Sub(started),
		resolved.IncludesIncompletePeriod,
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

func (d *Datasource) queryReservationUtilization(
	ctx context.Context,
	query backend.DataQuery,
	model models.Query,
	resolved models.ResolvedDateRange,
	started time.Time,
	logger log.Logger,
) backend.DataResponse {
	if resolved.Empty {
		resultFrames, convertErr := frames.ConvertReservationUtilization(model, &awscostexplorer.GetReservationUtilizationOutput{})
		if convertErr != nil {
			return backend.ErrDataResponse(
				backend.StatusInternal,
				fmt.Sprintf("create empty AWS Reservation Utilization data frames: %v", convertErr),
			)
		}
		for _, frame := range resultFrames {
			frame.RefID = query.RefID
			if frame.Meta == nil {
				frame.Meta = &data.FrameMeta{}
			}
			frame.Meta.Notices = append(frame.Meta.Notices, data.Notice{
				Severity: data.NoticeSeverityInfo,
				Text:     models.NoCompletedPeriodsNotice,
			})
		}

		completedAt := d.now()
		metadata := NewQueryExecutionMetadata(
			&costexplorer.Execution{
				CacheResult: cache.ResultBypass,
				CacheTTL:    time.Duration(d.settings.CacheTTLSeconds) * time.Second,
			},
			completedAt,
			completedAt.Sub(started),
			false,
		)
		AttachQueryExecutionMetadata(resultFrames, metadata)
		fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
		logger.Info("Reservation Utilization query completed without an AWS request", fields...)
		return backend.DataResponse{Status: backend.StatusOK, Frames: resultFrames}
	}

	awsContext, cancel := context.WithTimeout(ctx, awsRequestTimeout)
	defer cancel()

	execution, err := d.service.ReservationUtilization(awsContext, d.settings, model, resolved.DateRange)
	if err != nil {
		failedAt := d.now()
		metadata := NewQueryExecutionMetadata(
			reservationUtilizationMetadataExecution(execution),
			failedAt,
			failedAt.Sub(started),
			resolved.IncludesIncompletePeriod,
		)
		fields := append(metadata.LogFields(), "error_category", "aws_reservation_utilization_request_failed")
		logger.Error("Reservation Utilization query failed", fields...)
		status := backend.StatusBadGateway
		if awsContext.Err() == context.DeadlineExceeded {
			status = backend.StatusTimeout
		}
		errorResponse := backend.ErrDataResponse(status, err.Error())
		errorFrames, frameErr := frames.ConvertReservationUtilization(model, &awscostexplorer.GetReservationUtilizationOutput{})
		if frameErr == nil {
			for _, frame := range errorFrames {
				frame.RefID = query.RefID
			}
			AttachQueryExecutionMetadata(errorFrames, metadata)
			errorResponse.Frames = errorFrames
		}
		return errorResponse
	}

	resultFrames, err := frames.ConvertReservationUtilization(model, execution.Output)
	if err != nil {
		logger.Error("data frame conversion failed", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("convert AWS response to data frames: %v", err))
	}
	for _, frame := range resultFrames {
		frame.RefID = query.RefID
	}

	completedAt := d.now()
	metadata := NewQueryExecutionMetadata(
		reservationUtilizationMetadataExecution(execution),
		completedAt,
		completedAt.Sub(started),
		resolved.IncludesIncompletePeriod,
	)
	AttachQueryExecutionMetadata(resultFrames, metadata)

	fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
	logger.Info("Reservation Utilization query completed", fields...)

	return backend.DataResponse{Status: backend.StatusOK, Frames: resultFrames}
}

func (d *Datasource) querySavingsPlansUtilization(
	ctx context.Context,
	query backend.DataQuery,
	model models.Query,
	resolved models.ResolvedDateRange,
	started time.Time,
	logger log.Logger,
) backend.DataResponse {
	if resolved.Empty {
		resultFrames, convertErr := frames.ConvertSavingsPlansUtilization(model, &awscostexplorer.GetSavingsPlansUtilizationOutput{})
		if convertErr != nil {
			return backend.ErrDataResponse(
				backend.StatusInternal,
				fmt.Sprintf("create empty AWS Savings Plans Utilization data frames: %v", convertErr),
			)
		}
		for _, frame := range resultFrames {
			frame.RefID = query.RefID
			if frame.Meta == nil {
				frame.Meta = &data.FrameMeta{}
			}
			frame.Meta.Notices = append(frame.Meta.Notices, data.Notice{
				Severity: data.NoticeSeverityInfo,
				Text:     models.NoCompletedPeriodsNotice,
			})
		}

		completedAt := d.now()
		metadata := NewQueryExecutionMetadata(
			&costexplorer.Execution{
				CacheResult: cache.ResultBypass,
				CacheTTL:    time.Duration(d.settings.CacheTTLSeconds) * time.Second,
			},
			completedAt,
			completedAt.Sub(started),
			false,
		)
		AttachQueryExecutionMetadata(resultFrames, metadata)
		fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
		logger.Info("Savings Plans Utilization query completed without an AWS request", fields...)
		return backend.DataResponse{Status: backend.StatusOK, Frames: resultFrames}
	}

	awsContext, cancel := context.WithTimeout(ctx, awsRequestTimeout)
	defer cancel()

	execution, err := d.service.SavingsPlansUtilization(awsContext, d.settings, model, resolved.DateRange)
	if err != nil {
		failedAt := d.now()
		metadata := NewQueryExecutionMetadata(
			savingsPlansUtilizationMetadataExecution(execution),
			failedAt,
			failedAt.Sub(started),
			resolved.IncludesIncompletePeriod,
		)
		fields := append(metadata.LogFields(), "error_category", "aws_savings_plans_utilization_request_failed")
		logger.Error("Savings Plans Utilization query failed", fields...)
		status := backend.StatusBadGateway
		if awsContext.Err() == context.DeadlineExceeded {
			status = backend.StatusTimeout
		}
		errorResponse := backend.ErrDataResponse(status, err.Error())
		errorFrames, frameErr := frames.ConvertSavingsPlansUtilization(model, &awscostexplorer.GetSavingsPlansUtilizationOutput{})
		if frameErr == nil {
			for _, frame := range errorFrames {
				frame.RefID = query.RefID
			}
			AttachQueryExecutionMetadata(errorFrames, metadata)
			errorResponse.Frames = errorFrames
		}
		return errorResponse
	}

	resultFrames, err := frames.ConvertSavingsPlansUtilization(model, execution.Output)
	if err != nil {
		logger.Error("data frame conversion failed", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("convert AWS response to data frames: %v", err))
	}
	for _, frame := range resultFrames {
		frame.RefID = query.RefID
	}

	completedAt := d.now()
	metadata := NewQueryExecutionMetadata(
		savingsPlansUtilizationMetadataExecution(execution),
		completedAt,
		completedAt.Sub(started),
		resolved.IncludesIncompletePeriod,
	)
	AttachQueryExecutionMetadata(resultFrames, metadata)

	fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
	logger.Info("Savings Plans Utilization query completed", fields...)

	return backend.DataResponse{Status: backend.StatusOK, Frames: resultFrames}
}

func (d *Datasource) queryReservationCoverage(
	ctx context.Context,
	query backend.DataQuery,
	model models.Query,
	resolved models.ResolvedDateRange,
	started time.Time,
	logger log.Logger,
) backend.DataResponse {
	if resolved.Empty {
		resultFrames, convertErr := frames.ConvertReservationCoverage(model, &awscostexplorer.GetReservationCoverageOutput{})
		if convertErr != nil {
			return backend.ErrDataResponse(
				backend.StatusInternal,
				fmt.Sprintf("create empty AWS Reservation Coverage data frames: %v", convertErr),
			)
		}
		for _, frame := range resultFrames {
			frame.RefID = query.RefID
			if frame.Meta == nil {
				frame.Meta = &data.FrameMeta{}
			}
			frame.Meta.Notices = append(frame.Meta.Notices, data.Notice{
				Severity: data.NoticeSeverityInfo,
				Text:     models.NoCompletedPeriodsNotice,
			})
		}

		completedAt := d.now()
		metadata := NewQueryExecutionMetadata(
			&costexplorer.Execution{
				CacheResult: cache.ResultBypass,
				CacheTTL:    time.Duration(d.settings.CacheTTLSeconds) * time.Second,
			},
			completedAt,
			completedAt.Sub(started),
			false,
		)
		AttachQueryExecutionMetadata(resultFrames, metadata)
		fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
		logger.Info("Reservation Coverage query completed without an AWS request", fields...)
		return backend.DataResponse{Status: backend.StatusOK, Frames: resultFrames}
	}

	awsContext, cancel := context.WithTimeout(ctx, awsRequestTimeout)
	defer cancel()

	execution, err := d.service.ReservationCoverage(awsContext, d.settings, model, resolved.DateRange)
	if err != nil {
		failedAt := d.now()
		metadata := NewQueryExecutionMetadata(
			reservationCoverageMetadataExecution(execution),
			failedAt,
			failedAt.Sub(started),
			resolved.IncludesIncompletePeriod,
		)
		fields := append(metadata.LogFields(), "error_category", "aws_reservation_coverage_request_failed")
		logger.Error("Reservation Coverage query failed", fields...)
		status := backend.StatusBadGateway
		if awsContext.Err() == context.DeadlineExceeded {
			status = backend.StatusTimeout
		}
		errorResponse := backend.ErrDataResponse(status, err.Error())
		errorFrames, frameErr := frames.ConvertReservationCoverage(model, &awscostexplorer.GetReservationCoverageOutput{})
		if frameErr == nil {
			for _, frame := range errorFrames {
				frame.RefID = query.RefID
			}
			AttachQueryExecutionMetadata(errorFrames, metadata)
			errorResponse.Frames = errorFrames
		}
		return errorResponse
	}

	resultFrames, err := frames.ConvertReservationCoverage(model, execution.Output)
	if err != nil {
		logger.Error("data frame conversion failed", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("convert AWS response to data frames: %v", err))
	}
	for _, frame := range resultFrames {
		frame.RefID = query.RefID
	}

	completedAt := d.now()
	metadata := NewQueryExecutionMetadata(
		reservationCoverageMetadataExecution(execution),
		completedAt,
		completedAt.Sub(started),
		resolved.IncludesIncompletePeriod,
	)
	AttachQueryExecutionMetadata(resultFrames, metadata)

	fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
	logger.Info("Reservation Coverage query completed", fields...)

	return backend.DataResponse{Status: backend.StatusOK, Frames: resultFrames}
}

func (d *Datasource) querySavingsPlansCoverage(
	ctx context.Context,
	query backend.DataQuery,
	model models.Query,
	resolved models.ResolvedDateRange,
	started time.Time,
	logger log.Logger,
) backend.DataResponse {
	if resolved.Empty {
		resultFrames, convertErr := frames.ConvertSavingsPlansCoverage(model, &awscostexplorer.GetSavingsPlansCoverageOutput{})
		if convertErr != nil {
			return backend.ErrDataResponse(
				backend.StatusInternal,
				fmt.Sprintf("create empty AWS Savings Plans Coverage data frames: %v", convertErr),
			)
		}
		for _, frame := range resultFrames {
			frame.RefID = query.RefID
			if frame.Meta == nil {
				frame.Meta = &data.FrameMeta{}
			}
			frame.Meta.Notices = append(frame.Meta.Notices, data.Notice{
				Severity: data.NoticeSeverityInfo,
				Text:     models.NoCompletedPeriodsNotice,
			})
		}

		completedAt := d.now()
		metadata := NewQueryExecutionMetadata(
			&costexplorer.Execution{
				CacheResult: cache.ResultBypass,
				CacheTTL:    time.Duration(d.settings.CacheTTLSeconds) * time.Second,
			},
			completedAt,
			completedAt.Sub(started),
			false,
		)
		AttachQueryExecutionMetadata(resultFrames, metadata)
		fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
		logger.Info("Savings Plans Coverage query completed without an AWS request", fields...)
		return backend.DataResponse{Status: backend.StatusOK, Frames: resultFrames}
	}

	awsContext, cancel := context.WithTimeout(ctx, awsRequestTimeout)
	defer cancel()

	execution, err := d.service.SavingsPlansCoverage(awsContext, d.settings, model, resolved.DateRange)
	if err != nil {
		failedAt := d.now()
		metadata := NewQueryExecutionMetadata(
			savingsPlansCoverageMetadataExecution(execution),
			failedAt,
			failedAt.Sub(started),
			resolved.IncludesIncompletePeriod,
		)
		fields := append(metadata.LogFields(), "error_category", "aws_savings_plans_coverage_request_failed")
		logger.Error("Savings Plans Coverage query failed", fields...)
		status := backend.StatusBadGateway
		if awsContext.Err() == context.DeadlineExceeded {
			status = backend.StatusTimeout
		}
		errorResponse := backend.ErrDataResponse(status, err.Error())
		errorFrames, frameErr := frames.ConvertSavingsPlansCoverage(model, &awscostexplorer.GetSavingsPlansCoverageOutput{})
		if frameErr == nil {
			for _, frame := range errorFrames {
				frame.RefID = query.RefID
			}
			AttachQueryExecutionMetadata(errorFrames, metadata)
			errorResponse.Frames = errorFrames
		}
		return errorResponse
	}

	resultFrames, err := frames.ConvertSavingsPlansCoverage(model, execution.Output)
	if err != nil {
		logger.Error("data frame conversion failed", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("convert AWS response to data frames: %v", err))
	}
	for _, frame := range resultFrames {
		frame.RefID = query.RefID
	}

	completedAt := d.now()
	metadata := NewQueryExecutionMetadata(
		savingsPlansCoverageMetadataExecution(execution),
		completedAt,
		completedAt.Sub(started),
		resolved.IncludesIncompletePeriod,
	)
	AttachQueryExecutionMetadata(resultFrames, metadata)

	fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
	logger.Info("Savings Plans Coverage query completed", fields...)

	return backend.DataResponse{Status: backend.StatusOK, Frames: resultFrames}
}

func (d *Datasource) queryReservationInventory(
	ctx context.Context,
	query backend.DataQuery,
	started time.Time,
	logger log.Logger,
) backend.DataResponse {
	awsContext, cancel := context.WithTimeout(ctx, awsRequestTimeout)
	defer cancel()

	execution, err := d.inventoryService.Reservations(awsContext, d.settings)
	completedAt := d.now()
	metadata := NewQueryExecutionMetadata(
		reservationInventoryMetadataExecution(execution),
		completedAt,
		completedAt.Sub(started),
		false,
	)
	if err != nil {
		fields := append(metadata.LogFields(), "error_category", "aws_reservation_inventory_request_failed")
		logger.Error("Reservation inventory query failed", fields...)
		status := backend.StatusBadGateway
		if awsContext.Err() == context.DeadlineExceeded {
			status = backend.StatusTimeout
		}
		return backend.ErrDataResponse(status, err.Error())
	}

	resultFrames := frames.ConvertReservationInventory(execution.Items, execution.Notices)
	for _, frame := range resultFrames {
		frame.RefID = query.RefID
	}
	AttachQueryExecutionMetadata(resultFrames, metadata)

	fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
	logger.Info("Reservation inventory query completed", fields...)
	return backend.DataResponse{Status: backend.StatusOK, Frames: resultFrames}
}

func (d *Datasource) querySavingsPlansInventory(
	ctx context.Context,
	query backend.DataQuery,
	started time.Time,
	logger log.Logger,
) backend.DataResponse {
	awsContext, cancel := context.WithTimeout(ctx, awsRequestTimeout)
	defer cancel()

	execution, err := d.inventoryService.SavingsPlans(awsContext, d.settings)
	completedAt := d.now()
	metadata := NewQueryExecutionMetadata(
		savingsPlansInventoryMetadataExecution(execution),
		completedAt,
		completedAt.Sub(started),
		false,
	)
	if err != nil {
		fields := append(metadata.LogFields(), "error_category", "aws_savings_plans_inventory_request_failed")
		logger.Error("Savings Plans inventory query failed", fields...)
		status := backend.StatusBadGateway
		if awsContext.Err() == context.DeadlineExceeded {
			status = backend.StatusTimeout
		}
		return backend.ErrDataResponse(status, err.Error())
	}

	resultFrames := frames.ConvertSavingsPlansInventory(execution.Plans)
	for _, frame := range resultFrames {
		frame.RefID = query.RefID
	}
	AttachQueryExecutionMetadata(resultFrames, metadata)

	fields := append(metadata.LogFields(), "frame_count", len(resultFrames))
	logger.Info("Savings Plans inventory query completed", fields...)
	return backend.DataResponse{Status: backend.StatusOK, Frames: resultFrames}
}

// reservationInventoryMetadataExecution and savingsPlansInventoryMetadataExecution
// adapt the inventory package's Execution shape to the costexplorer.Execution
// fields NewQueryExecutionMetadata reads, the same technique used for the
// utilization query types below.
func reservationInventoryMetadataExecution(execution *inventory.ReservationInventoryExecution) *costexplorer.Execution {
	if execution == nil {
		return nil
	}
	return &costexplorer.Execution{
		CacheResult: execution.CacheResult,
		CacheAge:    execution.CacheAge,
		CacheTTL:    execution.CacheTTL,
		AWSDuration: execution.AWSDuration,
	}
}

func savingsPlansInventoryMetadataExecution(execution *inventory.SavingsPlansExecution) *costexplorer.Execution {
	if execution == nil {
		return nil
	}
	return &costexplorer.Execution{
		CacheResult: execution.CacheResult,
		CacheAge:    execution.CacheAge,
		CacheTTL:    execution.CacheTTL,
		AWSDuration: execution.AWSDuration,
	}
}

// reservationUtilizationMetadataExecution and
// savingsPlansUtilizationMetadataExecution adapt the utilization-specific
// Execution shapes to the costexplorer.Execution fields NewQueryExecutionMetadata
// reads, without changing that function's existing signature or tests.
func reservationUtilizationMetadataExecution(execution *costexplorer.ReservationUtilizationExecution) *costexplorer.Execution {
	if execution == nil {
		return nil
	}
	return &costexplorer.Execution{
		CacheResult: execution.CacheResult,
		CacheAge:    execution.CacheAge,
		CacheTTL:    execution.CacheTTL,
		Pages:       execution.Pages,
		AWSDuration: execution.AWSDuration,
	}
}

func savingsPlansUtilizationMetadataExecution(execution *costexplorer.SavingsPlansUtilizationExecution) *costexplorer.Execution {
	if execution == nil {
		return nil
	}
	return &costexplorer.Execution{
		CacheResult: execution.CacheResult,
		CacheAge:    execution.CacheAge,
		CacheTTL:    execution.CacheTTL,
		AWSDuration: execution.AWSDuration,
	}
}

func reservationCoverageMetadataExecution(execution *costexplorer.ReservationCoverageExecution) *costexplorer.Execution {
	if execution == nil {
		return nil
	}
	return &costexplorer.Execution{
		CacheResult: execution.CacheResult,
		CacheAge:    execution.CacheAge,
		CacheTTL:    execution.CacheTTL,
		Pages:       execution.Pages,
		AWSDuration: execution.AWSDuration,
	}
}

func savingsPlansCoverageMetadataExecution(execution *costexplorer.SavingsPlansCoverageExecution) *costexplorer.Execution {
	if execution == nil {
		return nil
	}
	return &costexplorer.Execution{
		CacheResult: execution.CacheResult,
		CacheAge:    execution.CacheAge,
		CacheTTL:    execution.CacheTTL,
		Pages:       execution.Pages,
		AWSDuration: execution.AWSDuration,
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
		return healthFailure(credentialHealthFailureMessage), nil
	}

	now := d.now().UTC()
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
		return healthFailure(costExplorerHealthFailureMessage), nil
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

func (d *Datasource) CallResource(
	ctx context.Context,
	req *backend.CallResourceRequest,
	sender backend.CallResourceResponseSender,
) error {
	return d.resourceHandler.CallResource(ctx, req, sender)
}

func (d *Datasource) buildResourceHandler() backend.CallResourceHandler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dimension-values", d.handleDimensionValues)
	mux.HandleFunc("GET /tag-keys", d.handleTagKeys)
	mux.HandleFunc("GET /tag-values", d.handleTagValues)
	return httpadapter.New(mux)
}

func (d *Datasource) handleDimensionValues(w http.ResponseWriter, r *http.Request) {
	if !d.ensureReady(w) {
		return
	}
	dimension := r.URL.Query().Get("dimension")
	if _, ok := costexplorer.FilterableDimensions[dimension]; !ok {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("dimension %q does not support value lookup", dimension))
		return
	}

	dateRange, err := d.lookupDateRange()
	if err != nil {
		d.logger.FromContext(r.Context()).Error("resolve lookup date range failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to resolve lookup date range")
		return
	}

	values, err := d.service.DimensionValues(r.Context(), d.settings, dimension, r.URL.Query().Get("search"), dateRange)
	if err != nil {
		d.logger.FromContext(r.Context()).Warn("dimension value lookup failed", "dimension", dimension, "error", err)
		writeJSONError(w, http.StatusBadGateway, "dimension value lookup failed")
		return
	}
	writeJSONValues(w, values)
}

func (d *Datasource) handleTagKeys(w http.ResponseWriter, r *http.Request) {
	if !d.ensureReady(w) {
		return
	}

	dateRange, err := d.lookupDateRange()
	if err != nil {
		d.logger.FromContext(r.Context()).Error("resolve lookup date range failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to resolve lookup date range")
		return
	}

	values, err := d.service.TagKeys(r.Context(), d.settings, r.URL.Query().Get("search"), dateRange)
	if err != nil {
		d.logger.FromContext(r.Context()).Warn("tag key lookup failed", "error", err)
		writeJSONError(w, http.StatusBadGateway, "tag key lookup failed")
		return
	}
	writeJSONValues(w, values)
}

func (d *Datasource) handleTagValues(w http.ResponseWriter, r *http.Request) {
	if !d.ensureReady(w) {
		return
	}
	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSONError(w, http.StatusBadRequest, "key is required")
		return
	}

	dateRange, err := d.lookupDateRange()
	if err != nil {
		d.logger.FromContext(r.Context()).Error("resolve lookup date range failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to resolve lookup date range")
		return
	}

	values, err := d.service.TagValues(r.Context(), d.settings, key, r.URL.Query().Get("search"), dateRange)
	if err != nil {
		d.logger.FromContext(r.Context()).Warn("tag value lookup failed", "key", key, "error", err)
		writeJSONError(w, http.StatusBadGateway, "tag value lookup failed")
		return
	}
	writeJSONValues(w, values)
}

// ensureReady reports whether the data source initialized successfully,
// writing a JSON error response and returning false otherwise.
func (d *Datasource) ensureReady(w http.ResponseWriter) bool {
	if d.initializationErr != nil {
		writeJSONError(w, http.StatusInternalServerError, d.initializationErr.Error())
		return false
	}
	if d.service == nil {
		writeJSONError(w, http.StatusInternalServerError, "data source was not initialized")
		return false
	}
	return true
}

// lookupDateRange returns the fixed trailing window used for dimension-value
// and tag lookups, independent of any panel's dashboard time range.
func (d *Datasource) lookupDateRange() (models.DateRange, error) {
	to := d.now().UTC()
	from := to.AddDate(0, 0, -lookupWindowDays)
	return models.CostExplorerDateRange(from, to)
}

func writeJSONValues(w http.ResponseWriter, values []string) {
	writeJSON(w, http.StatusOK, map[string][]string{"values": values})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func healthError(err error) *backend.CheckHealthResult {
	return healthFailure(err.Error())
}

func healthFailure(message string) *backend.CheckHealthResult {
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusError,
		Message: message,
	}
}
