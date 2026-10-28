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

// ReservationCoverageExecution mirrors Execution's shape for
// GetReservationCoverage so Query Inspector metadata works the same way it
// does for GetCostAndUsage.
type ReservationCoverageExecution struct {
	Output      *awscostexplorer.GetReservationCoverageOutput
	CacheResult cache.Result
	CacheAge    time.Duration
	CacheTTL    time.Duration
	Pages       int
	AWSDuration time.Duration
}

func (e ReservationCoverageExecution) CacheStatus() cache.Result {
	return (cache.Details{Result: e.CacheResult}).MetadataStatus()
}

type cachedReservationCoverageResponse struct {
	Output      *awscostexplorer.GetReservationCoverageOutput
	Pages       int
	AWSDuration time.Duration
}

// ReservationCoverage calls AWS Cost Explorer's GetReservationCoverage,
// reusing this service's cache and the same dimension filter as
// GetCostAndUsage (Service/LinkedAccount/Region only; Query.Validate already
// rejects tag filters and grouping for this query type).
func (s *Service) ReservationCoverage(
	ctx context.Context,
	settings models.PluginSettings,
	query models.Query,
	dateRange models.DateRange,
) (*ReservationCoverageExecution, error) {
	query.ApplyDefaults()
	if err := query.Validate(); err != nil {
		return nil, err
	}

	timePeriod, err := dateInterval(dateRange)
	if err != nil {
		return nil, err
	}
	input := &awscostexplorer.GetReservationCoverageInput{
		TimePeriod:  timePeriod,
		Granularity: types.Granularity(query.Granularity),
		Filter:      buildFilter(query.Filter),
		// Hour and Cost populate CoverageHours/CoverageCost, the two curated
		// measurement groups this plugin exposes. Unit (CoverageNormalizedUnits)
		// is intentionally not requested -- an intentional, documented scope
		// boundary, consistent with Utilization's curated lists.
		Metrics: []string{"Hour", "Cost"},
	}

	key, err := lookupCacheKey(
		settings.CredentialContext(),
		[]string{"reservation-coverage", query.Filter.Service, query.Filter.LinkedAccount, query.Filter.Region},
		dateRange,
	)
	if err != nil {
		return nil, err
	}

	value, cacheDetails, err := s.cache.GetOrLoad(ctx, key, func(loadContext context.Context) (any, error) {
		started := time.Now()
		output, pages, loadErr := s.fetchAllReservationCoverage(loadContext, input)
		return &cachedReservationCoverageResponse{
			Output:      output,
			Pages:       pages,
			AWSDuration: time.Since(started),
		}, loadErr
	})

	cached, ok := value.(*cachedReservationCoverageResponse)
	if !ok || cached == nil {
		if err != nil {
			return &ReservationCoverageExecution{
				CacheResult: cacheDetails.Result,
				CacheAge:    cacheDetails.Age,
				CacheTTL:    cacheDetails.TTL,
			}, ClassifyError(err)
		}
		return nil, fmt.Errorf("cache returned an invalid AWS Reservation Coverage response")
	}

	awsDuration := time.Duration(0)
	pages := 0
	if cacheDetails.Result == cache.ResultMiss {
		awsDuration = cached.AWSDuration
		pages = cached.Pages
	}
	execution := &ReservationCoverageExecution{
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
		return nil, fmt.Errorf("cache returned an invalid AWS Reservation Coverage response")
	}

	return execution, nil
}

func (s *Service) fetchAllReservationCoverage(
	ctx context.Context,
	baseInput *awscostexplorer.GetReservationCoverageInput,
) (*awscostexplorer.GetReservationCoverageOutput, int, error) {
	combined := &awscostexplorer.GetReservationCoverageOutput{}
	seenTokens := make(map[string]struct{})
	var token *string

	for page := 1; page <= maxPages; page++ {
		input := *baseInput
		input.NextPageToken = token

		output, err := s.client.GetReservationCoverage(ctx, &input)
		if err != nil {
			if isDataUnavailable(err) {
				return combined, page, nil
			}
			return nil, page, err
		}
		if output == nil {
			return nil, page, fmt.Errorf("AWS returned an empty GetReservationCoverage response")
		}

		combined.CoveragesByTime = append(combined.CoveragesByTime, output.CoveragesByTime...)
		if page == 1 {
			combined.Total = output.Total
		}

		if output.NextPageToken == nil || *output.NextPageToken == "" {
			return combined, page, nil
		}
		if _, duplicate := seenTokens[*output.NextPageToken]; duplicate {
			return nil, page, fmt.Errorf("AWS returned a repeated pagination token")
		}
		seenTokens[*output.NextPageToken] = struct{}{}
		token = output.NextPageToken
	}

	return nil, maxPages, fmt.Errorf("response from AWS Reservation Coverage exceeded the %d-page safety limit", maxPages)
}
