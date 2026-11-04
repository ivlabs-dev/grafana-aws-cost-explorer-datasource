package inventory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/awsclient"
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
func (s *Service) Reservations(ctx context.Context, settings models.PluginSettings, allRegions bool) (*ReservationInventoryExecution, error) {
	subject := reservationInventoryCacheSubject
	if allRegions {
		subject += "-all-regions"
	}
	key, err := cacheKey(settings.CredentialContext(), subject)
	if err != nil {
		return nil, err
	}

	value, details, err := s.cache.GetOrLoad(ctx, key, func(loadContext context.Context) (any, error) {
		started := time.Now()
		items, notices, loadErr := s.fetchAllReservations(loadContext, settings.Region, allRegions)
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

// maxConcurrentReservationCalls bounds in-flight AWS calls. An all-regions
// query is up to regions x five services (roughly 85 calls), and this keeps
// that from bursting into AWS throttling.
const maxConcurrentReservationCalls = 16

type reservationTarget struct {
	region  string
	clients awsclient.ReservationClients
}

type reservationJob struct {
	service string
	region  string
	fetch   func(context.Context) ([]ReservedItem, error)
}

type reservationJobResult struct {
	items []ReservedItem
	err   error
}

func (s *Service) fetchAllReservations(
	ctx context.Context,
	homeRegion string,
	allRegions bool,
) ([]ReservedItem, []data.Notice, error) {
	targets, notices, err := s.reservationTargets(ctx, homeRegion, allRegions)
	if err != nil {
		return nil, nil, err
	}

	jobs := make([]reservationJob, 0, len(targets)*5)
	for _, target := range targets {
		clients := target.clients
		jobs = append(jobs,
			reservationJob{ServiceEC2, target.region, func(c context.Context) ([]ReservedItem, error) { return fetchEC2(c, clients.EC2) }},
			reservationJob{ServiceRDS, target.region, func(c context.Context) ([]ReservedItem, error) { return fetchRDS(c, clients.RDS) }},
			reservationJob{ServiceElastiCache, target.region, func(c context.Context) ([]ReservedItem, error) {
				return fetchElastiCache(c, clients.ElastiCache)
			}},
			reservationJob{ServiceRedshift, target.region, func(c context.Context) ([]ReservedItem, error) {
				return fetchRedshift(c, clients.Redshift)
			}},
			reservationJob{ServiceOpenSearch, target.region, func(c context.Context) ([]ReservedItem, error) {
				return fetchOpenSearch(c, clients.OpenSearch)
			}},
		)
	}

	results := make([]reservationJobResult, len(jobs))
	semaphore := make(chan struct{}, maxConcurrentReservationCalls)
	var waitGroup sync.WaitGroup
	for i, job := range jobs {
		waitGroup.Add(1)
		go func(i int, job reservationJob) {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			items, fetchErr := job.fetch(ctx)
			for j := range items {
				items[j].Region = job.region
			}
			results[i] = reservationJobResult{items: items, err: fetchErr}
		}(i, job)
	}
	waitGroup.Wait()

	var combined []ReservedItem
	succeeded := 0
	hardFailures := 0
	// Failures are grouped per service so an all-regions query doesn't emit
	// one notice per region.
	denied := map[string][]string{}
	failed := map[string][]string{}
	failedErr := map[string]error{}
	for i, result := range results {
		job := jobs[i]
		if result.err == nil {
			combined = append(combined, result.items...)
			succeeded++
			continue
		}
		if isAccessDenied(result.err) {
			denied[job.service] = append(denied[job.service], job.region)
			continue
		}
		if !isAWSAPIError(result.err) {
			return nil, nil, classifyError(job.service, result.err)
		}
		hardFailures++
		failed[job.service] = append(failed[job.service], job.region)
		if _, ok := failedErr[job.service]; !ok {
			failedErr[job.service] = classifyError(job.service, result.err)
		}
	}

	for _, service := range reservationServices {
		if regions := denied[service]; len(regions) > 0 {
			// Still excluded from the merged list (this permission is
			// optional, like every other optional permission this plugin
			// uses), but an Info notice -- rather than silence -- so "zero
			// reservations shown" and "this service's permission wasn't
			// granted" aren't indistinguishable from the panel.
			text := fmt.Sprintf(
				"%s reservations are not shown: grant %s to include them.",
				service, requiredActionFor(service),
			)
			if len(regions) < len(targets) {
				// Denied in only some regions usually means a region-level
				// restriction (such as an SCP), not a missing permission.
				text = fmt.Sprintf(
					"%s reservations are not shown for %s: access was denied (a missing %s permission or a region restriction).",
					service, strings.Join(regions, ", "), requiredActionFor(service),
				)
			}
			notices = append(notices, data.Notice{Severity: data.NoticeSeverityInfo, Text: text})
		}
		if regions := failed[service]; len(regions) > 0 {
			text := fmt.Sprintf("Could not retrieve %s reservations: %v", service, failedErr[service])
			if allRegions {
				text = fmt.Sprintf("Could not retrieve %s reservations in %s: %v", service, strings.Join(regions, ", "), failedErr[service])
			}
			notices = append(notices, data.Notice{Severity: data.NoticeSeverityWarning, Text: text})
		}
	}

	if succeeded == 0 && hardFailures == len(jobs) {
		return nil, nil, fmt.Errorf("all reservation inventory sources failed; see per-service notices")
	}

	return combined, notices, nil
}

// reservationTargets resolves which regions to query. The configured
// (home) region always comes first and always uses the home clients.
// Region discovery failing never fails the query: it falls back to the
// home region with a notice, because ec2:DescribeRegions is optional.
func (s *Service) reservationTargets(
	ctx context.Context,
	homeRegion string,
	allRegions bool,
) ([]reservationTarget, []data.Notice, error) {
	home := reservationTarget{region: homeRegion, clients: s.home}
	if !allRegions || s.regions == nil || s.clientsForRegion == nil {
		return []reservationTarget{home}, nil, nil
	}

	output, err := s.regions.DescribeRegions(ctx, &ec2.DescribeRegionsInput{})
	if err != nil {
		if !isAWSAPIError(err) {
			return nil, nil, classifyError("EC2 region discovery", err)
		}
		text := fmt.Sprintf(
			"Only the configured region (%s) is shown: could not list regions (%v).",
			homeRegion, classifyError("EC2 region", err),
		)
		if isAccessDenied(err) {
			text = fmt.Sprintf(
				"Only the configured region (%s) is shown: grant ec2:DescribeRegions to include all regions.",
				homeRegion,
			)
		}
		severity := data.NoticeSeverityWarning
		if isAccessDenied(err) {
			severity = data.NoticeSeverityInfo
		}
		return []reservationTarget{home}, []data.Notice{{Severity: severity, Text: text}}, nil
	}

	targets := []reservationTarget{home}
	seen := map[string]struct{}{homeRegion: {}}
	for _, region := range output.Regions {
		name := stringValue(region.RegionName)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		targets = append(targets, reservationTarget{region: name, clients: s.clientsForRegion(name)})
	}
	return targets, nil, nil
}

var reservationServices = []string{ServiceEC2, ServiceRDS, ServiceElastiCache, ServiceRedshift, ServiceOpenSearch}

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
