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

// SavingsPlansCoverageExecution mirrors Execution's shape for
// GetSavingsPlansCoverage. Unlike GetSavingsPlansUtilization, this API does
// paginate (its default MaxResults is 20 rows, one row per ungrouped
// period), so Pages behaves the same way it does for GetCostAndUsage.
type SavingsPlansCoverageExecution struct {
	Output      *awscostexplorer.GetSavingsPlansCoverageOutput
	CacheResult cache.Result
	CacheAge    time.Duration
	CacheTTL    time.Duration
	Pages       int
	AWSDuration time.Duration
}

func (e SavingsPlansCoverageExecution) CacheStatus() cache.Result {
	return (cache.Details{Result: e.CacheResult}).MetadataStatus()
}

type cachedSavingsPlansCoverageResponse struct {
	Output      *awscostexplorer.GetSavingsPlansCoverageOutput
	Pages       int
	AWSDuration time.Duration
}

// SavingsPlansCoverage calls AWS Cost Explorer's GetSavingsPlansCoverage,
// reusing this service's cache. The API doesn't support a SERVICE filter or
// any grouping in this plugin's usage; Query.Validate already rejects both
// for this query type.
func (s *Service) SavingsPlansCoverage(
	ctx context.Context,
	settings models.PluginSettings,
	query models.Query,
	dateRange models.DateRange,
) (*SavingsPlansCoverageExecution, error) {
	query.ApplyDefaults()
	if err := query.Validate(); err != nil {
		return nil, err
	}

	timePeriod, err := dateInterval(dateRange)
	if err != nil {
		return nil, err
	}
	input := &awscostexplorer.GetSavingsPlansCoverageInput{
		TimePeriod:  timePeriod,
		Granularity: types.Granularity(query.Granularity),
		Filter:      buildFilter(query.Filter),
		Metrics:     []string{"SpendCoveredBySavingsPlans"},
	}

	key, err := lookupCacheKey(
		settings.CredentialContext(),
		[]string{"savings-plans-coverage", query.Filter.LinkedAccount, query.Filter.Region},
		dateRange,
	)
	if err != nil {
		return nil, err
	}

	value, cacheDetails, err := s.cache.GetOrLoad(ctx, key, func(loadContext context.Context) (any, error) {
		started := time.Now()
		output, pages, loadErr := s.fetchAllSavingsPlansCoverage(loadContext, input)
		return &cachedSavingsPlansCoverageResponse{
			Output:      output,
			Pages:       pages,
			AWSDuration: time.Since(started),
		}, loadErr
	})

	cached, ok := value.(*cachedSavingsPlansCoverageResponse)
	if !ok || cached == nil {
		if err != nil {
			return &SavingsPlansCoverageExecution{
				CacheResult: cacheDetails.Result,
				CacheAge:    cacheDetails.Age,
				CacheTTL:    cacheDetails.TTL,
			}, ClassifyError(err)
		}
		return nil, fmt.Errorf("cache returned an invalid AWS Savings Plans Coverage response")
	}

	awsDuration := time.Duration(0)
	pages := 0
	if cacheDetails.Result == cache.ResultMiss {
		awsDuration = cached.AWSDuration
		pages = cached.Pages
	}
	execution := &SavingsPlansCoverageExecution{
		Output:      cached.Output,
		CacheResult: cacheDetails.Result,
		CacheAge:    cacheDetails.Age,
		CacheTTL:    cacheDetails.TTL,
		Pages:       pages,
		AWSDuration: awsDuration,
	}
	if err != nil {
		return execution, ClassifyError(err)
	}
	if cached.Output == nil {
		return nil, fmt.Errorf("cache returned an invalid AWS Savings Plans Coverage response")
	}

	return execution, nil
}

func (s *Service) fetchAllSavingsPlansCoverage(
	ctx context.Context,
	baseInput *awscostexplorer.GetSavingsPlansCoverageInput,
) (*awscostexplorer.GetSavingsPlansCoverageOutput, int, error) {
	combined := &awscostexplorer.GetSavingsPlansCoverageOutput{}
	seenTokens := make(map[string]struct{})
	var token *string

	for page := 1; page <= maxPages; page++ {
		input := *baseInput
		input.NextToken = token

		output, err := s.client.GetSavingsPlansCoverage(ctx, &input)
		if err != nil {
			if isDataUnavailable(err) {
				return combined, page, nil
			}
			return nil, page, err
		}
		if output == nil {
			return nil, page, fmt.Errorf("AWS returned an empty GetSavingsPlansCoverage response")
		}

		combined.SavingsPlansCoverages = append(combined.SavingsPlansCoverages, output.SavingsPlansCoverages...)

		if output.NextToken == nil || *output.NextToken == "" {
			return combined, page, nil
		}
		if _, duplicate := seenTokens[*output.NextToken]; duplicate {
			return nil, page, fmt.Errorf("AWS returned a repeated pagination token")
		}
		seenTokens[*output.NextToken] = struct{}{}
		token = output.NextToken
	}

	return nil, maxPages, fmt.Errorf("response from AWS Savings Plans Coverage exceeded the %d-page safety limit", maxPages)
}
