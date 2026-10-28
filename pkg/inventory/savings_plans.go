package inventory

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans/types"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

const (
	savingsPlansMaxPages     = 100
	savingsPlansCacheSubject = "savings-plans-inventory"
)

// SavingsPlansExecution is the result of SavingsPlans, including the cache
// and timing facts NewQueryExecutionMetadata needs to report on this query
// the same way as every other query type.
type SavingsPlansExecution struct {
	Execution
	Plans []SavingsPlan
}

// SavingsPlans lists every Savings Plan the account owns, across all
// regions and types. Unlike the Reservation inventory, there is only one
// underlying AWS permission here, so a missing-permission error is
// classified and surfaced like any other query failure rather than
// silently degraded -- there's no partial result to fall back to.
func (s *Service) SavingsPlans(ctx context.Context, settings models.PluginSettings) (*SavingsPlansExecution, error) {
	key, err := cacheKey(settings.CredentialContext(), savingsPlansCacheSubject)
	if err != nil {
		return nil, err
	}

	value, details, err := s.cache.GetOrLoad(ctx, key, func(loadContext context.Context) (any, error) {
		started := time.Now()
		plans, loadErr := s.fetchSavingsPlans(loadContext)
		if loadErr != nil {
			return nil, loadErr
		}
		return &savingsPlansCachedResponse{Plans: plans, AWSDuration: time.Since(started)}, nil
	})

	if err != nil {
		return &SavingsPlansExecution{Execution: Execution{CacheResult: details.Result, CacheAge: details.Age, CacheTTL: details.TTL}},
			classifyError("Savings Plans", err)
	}

	cached, ok := value.(*savingsPlansCachedResponse)
	if !ok || cached == nil {
		return nil, fmt.Errorf("cache returned an invalid Savings Plans inventory response")
	}

	awsDuration := time.Duration(0)
	if details.Result == cache.ResultMiss {
		awsDuration = cached.AWSDuration
	}
	return &SavingsPlansExecution{
		Execution: Execution{
			CacheResult: details.Result,
			CacheAge:    details.Age,
			CacheTTL:    details.TTL,
			AWSDuration: awsDuration,
		},
		Plans: cached.Plans,
	}, nil
}

type savingsPlansCachedResponse struct {
	Plans       []SavingsPlan
	AWSDuration time.Duration
}

func (s *Service) fetchSavingsPlans(ctx context.Context) ([]SavingsPlan, error) {
	if s.savingsPlans == nil {
		return nil, nil
	}

	var combined []SavingsPlan
	var token *string
	for page := 1; page <= savingsPlansMaxPages; page++ {
		output, err := s.savingsPlans.DescribeSavingsPlans(ctx, &savingsplans.DescribeSavingsPlansInput{
			NextToken: token,
		})
		if err != nil {
			return nil, err
		}
		for _, plan := range output.SavingsPlans {
			combined = append(combined, toSavingsPlan(plan))
		}
		if output.NextToken == nil || *output.NextToken == "" {
			return combined, nil
		}
		token = output.NextToken
	}
	return nil, fmt.Errorf("Savings Plans inventory exceeded the %d-page safety limit", savingsPlansMaxPages)
}

func toSavingsPlan(plan types.SavingsPlan) SavingsPlan {
	return SavingsPlan{
		ID:            stringValue(plan.SavingsPlanId),
		Type:          string(plan.SavingsPlanType),
		PaymentOption: string(plan.PaymentOption),
		Region:        stringValue(plan.Region),
		Commitment:    parseFloat(plan.Commitment),
		State:         string(plan.State),
		Start:         parseAWSTimestamp(plan.Start),
		End:           parseAWSTimestamp(plan.End),
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func parseFloat(value *string) float64 {
	if value == nil {
		return 0
	}
	parsed, err := strconv.ParseFloat(*value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

// parseAWSTimestamp parses the Savings Plans service's ISO-8601 string
// timestamps. An unparseable or absent value becomes a zero time rather
// than an error -- a single malformed field shouldn't fail the whole list.
func parseAWSTimestamp(value *string) time.Time {
	if value == nil {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, *value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
