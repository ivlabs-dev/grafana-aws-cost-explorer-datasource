// Package awsmock implements every AWS client interface this plugin uses
// (pkg/awsclient's Bundle) with synthetic, parameter-aware data instead of
// real AWS calls. It exists so the data source can be evaluated, demoed, or
// reviewed -- e.g. by a Grafana plugin catalog reviewer with no AWS account
// of their own -- without ever sharing real credentials. See
// pkg/models.AuthModeMock and pkg/awsclient's SDKFactory.New.
package awsmock

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// ResolveCredentials always succeeds immediately: there is no credential of
// any kind involved anywhere in mock mode. Used as the Bundle's
// ResolveCredentials so CheckHealth's existing logic runs unchanged.
func ResolveCredentials(context.Context) error {
	return nil
}

// serviceCost is one curated AWS service this mock generates grouped cost
// data for, with a baseline daily cost used to seed deterministic jitter.
// Includes "Tax" so the bundled dashboard's service breakdown panel (which
// splits tax onto its own axis) has something real to split.
type serviceCost struct {
	name string
	base float64
}

var mockServices = []serviceCost{
	{"Amazon Elastic Compute Cloud - Compute", 420},
	{"Amazon Relational Database Service", 180},
	{"Amazon Simple Storage Service", 65},
	{"Amazon ElastiCache", 40},
	{"Amazon CloudFront", 25},
	{"AWS Lambda", 12},
	{"Tax", 55},
}

var mockLinkedAccounts = []string{"111111111111", "222222222222", "333333333333"}
var mockRegions = []string{"us-east-1", "us-west-2", "eu-west-1"}

var mockTagKeys = []string{"Environment", "Team"}
var mockTagValues = map[string][]string{
	"Environment": {"production", "staging", "development"},
	"Team":        {"platform", "data", "growth"},
}

// labelsFor returns the synthetic group labels for an AWS Cost Explorer
// group-by dimension, capped at four values so a 2-dimension grouping's
// cross product stays small. Unrecognized dimensions (e.g. INSTANCE_TYPE,
// USAGE_TYPE) still get a small generic fallback rather than an empty
// response, so trying them in mock mode doesn't look broken.
func labelsFor(dimension string) []string {
	switch dimension {
	case "SERVICE":
		names := make([]string, 0, len(mockServices))
		for _, s := range mockServices {
			names = append(names, s.name)
		}
		return names
	case "LINKED_ACCOUNT":
		return mockLinkedAccounts
	case "REGION":
		return mockRegions
	default:
		return []string{"Bucket A", "Bucket B"}
	}
}

// baseCostFor returns a plausible per-day baseline for a group label,
// falling back to a generic value for labels not in mockServices (accounts,
// regions, and the generic fallback buckets).
func baseCostFor(label string) float64 {
	for _, s := range mockServices {
		if s.name == label {
			return s.base
		}
	}
	return 90
}

// jitter returns a small deterministic multiplier (0.85-1.15) seeded by two
// integers, so repeated calls for the same (seed, day) pair always agree --
// screenshots and demo dashboards stay stable across refreshes rather than
// flickering on every query.
func jitter(seed, day int) float64 {
	h := int64(seed)*2654435761 + int64(day)*40503
	if h < 0 {
		h = -h
	}
	return 0.85 + float64(h%300)/1000
}

func amount(v float64) *string {
	return aws.String(strconv.FormatFloat(v, 'f', 6, 64))
}

func decimal(v float64) *string {
	return aws.String(strconv.FormatFloat(v, 'f', 1, 64))
}

// walkPeriods returns the UTC start-of-period boundaries between start
// (inclusive) and end (exclusive) implied by a Cost-Explorer-style date-only
// "YYYY-MM-DD" range and granularity, matching how AWS itself buckets
// GetCostAndUsage/GetReservationUtilization/GetSavingsPlansUtilization/
// Get*Coverage responses. A malformed or empty range returns nil, the same
// "nothing to report" shape AWS returns for an empty period.
func walkPeriods(startStr, endStr string, granularity string) []time.Time {
	start, err1 := time.Parse(time.DateOnly, startStr)
	end, err2 := time.Parse(time.DateOnly, endStr)
	if err1 != nil || err2 != nil || !end.After(start) {
		return nil
	}

	var periods []time.Time
	if strings.EqualFold(granularity, "MONTHLY") {
		cur := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
		for cur.Before(end) {
			periods = append(periods, cur)
			cur = cur.AddDate(0, 1, 0)
		}
		return periods
	}

	for cur := start; cur.Before(end); cur = cur.AddDate(0, 0, 1) {
		periods = append(periods, cur)
	}
	return periods
}

func periodEnd(periodStart time.Time, granularity string) time.Time {
	if strings.EqualFold(granularity, "MONTHLY") {
		return periodStart.AddDate(0, 1, 0)
	}
	return periodStart.AddDate(0, 0, 1)
}

func dateOnly(t time.Time) string {
	return t.Format(time.DateOnly)
}

// daysFromNow is used by the inventory mocks to build start/end dates
// relative to the current time, so expiry/days-until-expiry always looks
// dynamically relevant no matter when the dashboard is viewed, rather than
// drifting stale against a fixed calendar date baked in at write time.
func daysFromNow(days int) time.Time {
	return time.Now().UTC().AddDate(0, 0, days)
}
