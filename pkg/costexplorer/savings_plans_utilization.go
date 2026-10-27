package costexplorer

import (
	"context"
	"fmt"
	"time"

	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

// SavingsPlansUtilizationExecution mirrors Execution's shape for
// GetSavingsPlansUtilization. This API never paginates (it has no
// NextPageToken), so Pages is always 1 on a cache miss.
type SavingsPlansUtilizationExecution struct {
	Output      *awscostexplorer.GetSavingsPlansUtilizationOutput
	CacheResult cache.Result
	CacheAge    time.Duration
	CacheTTL    time.Duration
	AWSDuration time.Duration
}

func (e SavingsPlansUtilizationExecution) CacheStatus() cache.Result {
	return (cache.Details{Result: e.CacheResult}).MetadataStatus()
}

type cachedSavingsPlansUtilizationResponse struct {
	Output      *awscostexplorer.GetSavingsPlansUtilizationOutput
	AWSDuration time.Duration
}

// SavingsPlansUtilization calls AWS Cost Explorer's
// GetSavingsPlansUtilization, reusing this service's cache. The API doesn't
// support a SERVICE filter or any grouping; Query.Validate already rejects
// both for this query type, and it ignores Query.Metric/GroupBy entirely.
func (s *Service) SavingsPlansUtilization(
	ctx context.Context,
	settings models.PluginSettings,
	query models.Query,
	dateRange models.DateRange,
) (*SavingsPlansUtilizationExecution, error) {
	query.ApplyDefaults()
	if err := query.Validate(); err != nil {
		return nil, err
	}

	timePeriod, err := dateInterval(dateRange)
	if err != nil {
		return nil, err
	}
	input := &awscostexplorer.GetSavingsPlansUtilizationInput{
		TimePeriod:  timePeriod,
		Granularity: types.Granularity(query.Granularity),
		Filter:      buildFilter(query.Filter),
	}

	key, err := lookupCacheKey(
		settings.CredentialContext(),
		[]string{"savings-plans-utilization", query.Filter.LinkedAccount, query.Filter.Region},
		dateRange,
	)
	if err != nil {
		return nil, err
	}

	value, cacheDetails, err := s.cache.GetOrLoad(ctx, key, func(loadContext context.Context) (any, error) {
		started := time.Now()
		output, loadErr := s.client.GetSavingsPlansUtilization(loadContext, input)
		if loadErr != nil {
			if isDataUnavailable(loadErr) {
				return &cachedSavingsPlansUtilizationResponse{
					Output:      &awscostexplorer.GetSavingsPlansUtilizationOutput{},
					AWSDuration: time.Since(started),
				}, nil
			}
			return nil, loadErr
		}
		if output == nil {
			return nil, fmt.Errorf("AWS returned an empty GetSavingsPlansUtilization response")
		}
		return &cachedSavingsPlansUtilizationResponse{
			Output:      output,
			AWSDuration: time.Since(started),
		}, nil
	})

	cached, ok := value.(*cachedSavingsPlansUtilizationResponse)
	if !ok || cached == nil {
		if err != nil {
			return &SavingsPlansUtilizationExecution{
				CacheResult: cacheDetails.Result,
				CacheAge:    cacheDetails.Age,
				CacheTTL:    cacheDetails.TTL,
			}, ClassifyError(err)
		}
		return nil, fmt.Errorf("cache returned an invalid AWS Savings Plans Utilization response")
	}

	awsDuration := time.Duration(0)
	if cacheDetails.Result == cache.ResultMiss {
		awsDuration = cached.AWSDuration
	}
	execution := &SavingsPlansUtilizationExecution{
		Output:      cached.Output,
		CacheResult: cacheDetails.Result,
		CacheAge:    cacheDetails.Age,
		CacheTTL:    cacheDetails.TTL,
		AWSDuration: awsDuration,
	}
	if err != nil {
		return execution, ClassifyError(err)
	}
	if cached.Output == nil {
		return nil, fmt.Errorf("cache returned an invalid AWS Savings Plans Utilization response")
	}

	return execution, nil
}
