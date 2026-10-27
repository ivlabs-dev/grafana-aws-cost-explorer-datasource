package costexplorer

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

// ReservationUtilizationExecution mirrors Execution's shape for
// GetReservationUtilization so Query Inspector metadata (cache status, AWS
// duration, page count) works the same way it does for GetCostAndUsage.
type ReservationUtilizationExecution struct {
	Output      *awscostexplorer.GetReservationUtilizationOutput
	CacheResult cache.Result
	CacheAge    time.Duration
	CacheTTL    time.Duration
	Pages       int
	AWSDuration time.Duration
}

func (e ReservationUtilizationExecution) CacheStatus() cache.Result {
	return (cache.Details{Result: e.CacheResult}).MetadataStatus()
}

type cachedReservationUtilizationResponse struct {
	Output      *awscostexplorer.GetReservationUtilizationOutput
	Pages       int
	AWSDuration time.Duration
}

// ReservationUtilization calls AWS Cost Explorer's GetReservationUtilization,
// reusing this service's cache and the same dimension filter as
// GetCostAndUsage (Service/LinkedAccount/Region only; Query.Validate already
// rejects tag filters for this query type).
func (s *Service) ReservationUtilization(
	ctx context.Context,
	settings models.PluginSettings,
	query models.Query,
	dateRange models.DateRange,
) (*ReservationUtilizationExecution, error) {
	query.ApplyDefaults()
	if err := query.Validate(); err != nil {
		return nil, err
	}

	input, err := buildReservationUtilizationInput(query, dateRange)
	if err != nil {
		return nil, err
	}
	key, err := lookupCacheKey(
		settings.CredentialContext(),
		[]string{"reservation-utilization", query.Filter.Service, query.Filter.LinkedAccount, query.Filter.Region, boolString(query.GroupBySubscription)},
		dateRange,
	)
	if err != nil {
		return nil, err
	}

	value, cacheDetails, err := s.cache.GetOrLoad(ctx, key, func(loadContext context.Context) (any, error) {
		started := time.Now()
		output, pages, loadErr := s.fetchAllReservationUtilization(loadContext, input)
		return &cachedReservationUtilizationResponse{
			Output:      output,
			Pages:       pages,
			AWSDuration: time.Since(started),
		}, loadErr
	})

	cached, ok := value.(*cachedReservationUtilizationResponse)
	if !ok || cached == nil {
		if err != nil {
			return &ReservationUtilizationExecution{
				CacheResult: cacheDetails.Result,
				CacheAge:    cacheDetails.Age,
				CacheTTL:    cacheDetails.TTL,
			}, ClassifyError(err)
		}
		return nil, fmt.Errorf("cache returned an invalid AWS Reservation Utilization response")
	}

	awsDuration := time.Duration(0)
	pages := 0
	if cacheDetails.Result == cache.ResultMiss {
		awsDuration = cached.AWSDuration
		pages = cached.Pages
	}
	execution := &ReservationUtilizationExecution{
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
		return nil, fmt.Errorf("cache returned an invalid AWS Reservation Utilization response")
	}

	return execution, nil
}

func buildReservationUtilizationInput(
	query models.Query,
	dateRange models.DateRange,
) (*awscostexplorer.GetReservationUtilizationInput, error) {
	timePeriod, err := dateInterval(dateRange)
	if err != nil {
		return nil, err
	}

	input := &awscostexplorer.GetReservationUtilizationInput{
		TimePeriod:  timePeriod,
		Granularity: types.Granularity(query.Granularity),
		Filter:      buildFilter(query.Filter),
	}
	if query.GroupBySubscription {
		input.GroupBy = []types.GroupDefinition{{
			Key:  aws.String(string(types.DimensionSubscriptionId)),
			Type: types.GroupDefinitionTypeDimension,
		}}
		// AWS rejects a request that sets both GroupBy and Granularity: the
		// response then aggregates each subscription over the whole
		// requested period instead of breaking it down by day/month.
		input.Granularity = ""
	}

	return input, nil
}

func (s *Service) fetchAllReservationUtilization(
	ctx context.Context,
	baseInput *awscostexplorer.GetReservationUtilizationInput,
) (*awscostexplorer.GetReservationUtilizationOutput, int, error) {
	combined := &awscostexplorer.GetReservationUtilizationOutput{}
	seenTokens := make(map[string]struct{})
	var token *string

	for page := 1; page <= maxPages; page++ {
		input := *baseInput
		input.NextPageToken = token

		output, err := s.client.GetReservationUtilization(ctx, &input)
		if err != nil {
			if isDataUnavailable(err) {
				return combined, page, nil
			}
			return nil, page, err
		}
		if output == nil {
			return nil, page, fmt.Errorf("AWS returned an empty GetReservationUtilization response")
		}

		combined.UtilizationsByTime = append(combined.UtilizationsByTime, output.UtilizationsByTime...)
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

	return nil, maxPages, fmt.Errorf("response from AWS Reservation Utilization exceeded the %d-page safety limit", maxPages)
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}
