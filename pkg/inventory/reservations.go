package inventory

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

const reservationInventoryCacheSubject = "reservation-inventory"

// ReservationInventoryExecution is the result of Reservations: the merged
// list across all five reservation-eligible services, plus any per-service
// Notices for services that failed with something other than a missing
// permission.
type ReservationInventoryExecution struct {
	Execution
	Items   []ReservedItem
	Notices []data.Notice
}

// Reservations lists every Reservation the account owns across EC2, RDS,
// ElastiCache, Redshift, and OpenSearch, fetching all five concurrently and
// merging the results. Each of the five AWS permissions this requires is
// optional and independent of the others:
//
//   - A service whose call fails with an access-denied class error is
//     silently excluded, the same as this plugin's other optional
//     permissions (e.g. ce:GetDimensionValues) falling back gracefully.
//   - A service whose call fails with any other AWS API error is also
//     excluded, but a warning Notice naming that service is attached so the
//     failure isn't invisible.
//   - A network or context error (not a classifiable AWS API error) fails
//     the whole query immediately, since it isn't safe to assume that's a
//     permission problem.
//   - If every service produced a real (non-access-denied) error, the
//     whole query fails rather than silently returning an empty list that
//     looks identical to "no reservations".
func (s *Service) Reservations(ctx context.Context, settings models.PluginSettings) (*ReservationInventoryExecution, error) {
	key, err := cacheKey(settings.CredentialContext(), reservationInventoryCacheSubject)
	if err != nil {
		return nil, err
	}

	value, details, err := s.cache.GetOrLoad(ctx, key, func(loadContext context.Context) (any, error) {
		started := time.Now()
		items, notices, loadErr := s.fetchAllReservations(loadContext)
		if loadErr != nil {
			return nil, loadErr
		}
		return &reservationInventoryCachedResponse{
			Items:       items,
			Notices:     notices,
			AWSDuration: time.Since(started),
		}, nil
	})

	if err != nil {
		return &ReservationInventoryExecution{
			Execution: Execution{CacheResult: details.Result, CacheAge: details.Age, CacheTTL: details.TTL},
		}, err
	}

	cached, ok := value.(*reservationInventoryCachedResponse)
	if !ok || cached == nil {
		return nil, fmt.Errorf("cache returned an invalid Reservation inventory response")
	}

	awsDuration := time.Duration(0)
	if details.Result == cache.ResultMiss {
		awsDuration = cached.AWSDuration
	}
	return &ReservationInventoryExecution{
		Execution: Execution{
			CacheResult: details.Result,
			CacheAge:    details.Age,
			CacheTTL:    details.TTL,
			AWSDuration: awsDuration,
		},
		Items:   cached.Items,
		Notices: cached.Notices,
	}, nil
}

type reservationInventoryCachedResponse struct {
	Items       []ReservedItem
	Notices     []data.Notice
	AWSDuration time.Duration
}

type reservationFetch struct {
	service string
	fetch   func(context.Context) ([]ReservedItem, error)
}

type serviceFetchResult struct {
	service string
	items   []ReservedItem
	err     error
}

func (s *Service) fetchAllReservations(ctx context.Context) ([]ReservedItem, []data.Notice, error) {
	fetchers := []reservationFetch{
		{ServiceEC2, s.fetchEC2},
		{ServiceRDS, s.fetchRDS},
		{ServiceElastiCache, s.fetchElastiCache},
		{ServiceRedshift, s.fetchRedshift},
		{ServiceOpenSearch, s.fetchOpenSearch},
	}

	results := make([]serviceFetchResult, len(fetchers))
	var waitGroup sync.WaitGroup
	for i, fetcher := range fetchers {
		waitGroup.Add(1)
		go func(i int, fetcher reservationFetch) {
			defer waitGroup.Done()
			items, err := fetcher.fetch(ctx)
			results[i] = serviceFetchResult{service: fetcher.service, items: items, err: err}
		}(i, fetcher)
	}
	waitGroup.Wait()

	var combined []ReservedItem
	var notices []data.Notice
	succeeded := 0
	hardFailures := 0
	for _, result := range results {
		if result.err == nil {
			combined = append(combined, result.items...)
			succeeded++
			continue
		}
		if isAccessDenied(result.err) {
			// Still excluded from the merged list (this permission is
			// optional, like every other optional permission this plugin
			// uses), but an Info notice -- rather than silence -- so "zero
			// reservations shown" and "this service's permission wasn't
			// granted" aren't indistinguishable from the panel.
			notices = append(notices, data.Notice{
				Severity: data.NoticeSeverityInfo,
				Text: fmt.Sprintf(
					"%s reservations are not shown: grant %s to include them.",
					result.service, requiredActionFor(result.service),
				),
			})
			continue
		}
		if !isAWSAPIError(result.err) {
			return nil, nil, classifyError(result.service, result.err)
		}
		hardFailures++
		notices = append(notices, data.Notice{
			Severity: data.NoticeSeverityWarning,
			Text:     fmt.Sprintf("Could not retrieve %s reservations: %v", result.service, classifyError(result.service, result.err)),
		})
	}

	if succeeded == 0 && hardFailures == len(fetchers) {
		return nil, nil, fmt.Errorf("all reservation inventory sources failed; see per-service notices")
	}

	return combined, notices, nil
}

// requiredActionFor names the specific optional IAM action a service's
// reservation Describe call needs, so a permission notice tells the user
// exactly what to grant rather than just "access denied".
func requiredActionFor(service string) string {
	switch service {
	case ServiceEC2:
		return "ec2:DescribeReservedInstances"
	case ServiceRDS:
		return "rds:DescribeReservedDBInstances"
	case ServiceElastiCache:
		return "elasticache:DescribeReservedCacheNodes"
	case ServiceRedshift:
		return "redshift:DescribeReservedNodes"
	case ServiceOpenSearch:
		return "es:DescribeReservedInstances"
	default:
		return "the relevant Describe permission"
	}
}
