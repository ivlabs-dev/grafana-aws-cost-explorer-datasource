// Package awsmock implements every AWS client interface this plugin uses
// (pkg/awsclient's Bundle) with synthetic, parameter-aware data instead of
// real AWS calls. It exists so the data source can be evaluated, demoed, or
// reviewed -- e.g. by a Grafana plugin catalog reviewer with no AWS account
// of their own -- without ever sharing real credentials. See
// pkg/models.AuthModeMock and pkg/awsclient's SDKFactory.New.
package awsmock

import (
	"context"
	"math"
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

// anchorDate is the fixed origin every mock series is measured from. Values
// are a pure function of the calendar date (never of "days since the query
// started"), so a given day looks identical no matter what time range or
// granularity a panel requests -- panels agree with each other, and
// screenshots and demo dashboards stay stable across refreshes.
var anchorDate = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// taxService is derived from the other services' spend rather than modeled
// on its own, so the bundled dashboard's service breakdown panel (which
// splits tax onto its own axis) has a realistic proportional series.
const (
	taxService = "Tax"
	taxRate    = 0.08
)

// serviceCost describes how one curated AWS service's daily cost behaves:
// a baseline that grows slowly, a slow ~monthly wave, a weekday/weekend
// shape, day-to-day noise and (for bursty services) occasional spikes.
type serviceCost struct {
	name       string
	base       float64 // USD/day at the anchor date
	growth     float64 // fractional growth per day
	weekend    float64 // multiplier applied on Saturday/Sunday (1 = none)
	wave       float64 // amplitude of a slow ~30 day wave
	noise      float64 // amplitude of day-to-day noise
	spikeEvery float64 // chance a given day spikes (0 = never)
	spikeMult  float64 // extra multiplier on a spike day
}

var mockServices = []serviceCost{
	{name: "Amazon Elastic Compute Cloud - Compute", base: 420, growth: 0.0006, weekend: 0.85, wave: 0.04, noise: 0.02},
	{name: "Amazon Relational Database Service", base: 180, growth: 0.0003, weekend: 0.98, wave: 0.01, noise: 0.01},
	{name: "Amazon Simple Storage Service", base: 65, growth: 0.0012, weekend: 1, wave: 0.01, noise: 0.01},
	{name: "Amazon ElastiCache", base: 40, growth: 0, weekend: 1, wave: 0.02, noise: 0.015},
	{name: "Amazon CloudFront", base: 25, growth: 0.0004, weekend: 1.08, wave: 0.25, noise: 0.06},
	{name: "AWS Lambda", base: 12, growth: 0.0008, weekend: 0.7, wave: 0.1, noise: 0.15, spikeEvery: 0.06, spikeMult: 1.2},
	{name: taxService},
}

var mockLinkedAccounts = []string{"111111111111", "222222222222", "333333333333"}
var mockRegions = []string{"us-east-1", "us-west-2", "eu-west-1"}

var mockTagKeys = []string{"Environment", "Team"}
var mockTagValues = map[string][]string{
	"Environment": {"production", "staging", "development"},
	"Team":        {"platform", "data", "growth"},
}

// baseShares is each non-SERVICE group-by dimension's share of total spend,
// in labelsFor order. Shares sum to 1, so any grouping reconciles with the
// ungrouped total.
var baseShares = map[string][]float64{
	"LINKED_ACCOUNT": {0.55, 0.30, 0.15},
	"REGION":         {0.62, 0.26, 0.12},
	"Environment":    {0.58, 0.30, 0.12},
	"Team":           {0.45, 0.35, 0.20},
}

// labelsFor returns the synthetic group labels for an AWS Cost Explorer
// group-by dimension or cost allocation tag key, capped at four values so a
// 2-dimension grouping's cross product stays small. Unrecognized dimensions
// (e.g. INSTANCE_TYPE, USAGE_TYPE) still get a small generic fallback rather
// than an empty response, so trying them in mock mode doesn't look broken.
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
	}
	if values, ok := mockTagValues[dimension]; ok {
		return values
	}
	return []string{"Bucket A", "Bucket B"}
}

// dayNumber is the whole number of days between anchorDate and t.
func dayNumber(t time.Time) int {
	return int(t.Sub(anchorDate).Hours() / 24)
}

func isWeekend(t time.Time) bool {
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

// bits64 returns the two's-complement bit pattern of n as a uint64 without an
// unchecked signed-to-unsigned conversion: negative n maps to ^(-n-1).
func bits64(n int) uint64 {
	if n >= 0 {
		return uint64(n)
	}
	m := -(n + 1)
	if m < 0 { // unreachable: n < 0 implies m >= 0
		return 0
	}
	return ^uint64(m)
}

// hash64 is a splitmix64 finalizer: a stateless, well-mixed deterministic
// hash. Explicit uint64 arithmetic keeps results identical on 32-bit builds.
func hash64(seed, day int) uint64 {
	z := bits64(seed)<<32 ^ bits64(day)
	z += 0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// unit returns a deterministic value in [0, 1) for a (seed, day) pair.
func unit(seed, day int) float64 {
	return float64(hash64(seed, day)>>11) / float64(1<<53)
}

// noise returns a deterministic value in [-1, 1) for a (seed, day) pair.
func noise(seed, day int) float64 {
	return unit(seed, day)*2 - 1
}

// phase returns a stable per-seed offset in radians for slow waves, so
// different series don't all peak on the same day.
func phase(seed int) float64 {
	return unit(seed, -1) * 2 * math.Pi
}

// dailyCost is one service's cost on one day, before tax.
func (s serviceCost) dailyCost(seed int, t time.Time) float64 {
	d := dayNumber(t)
	trend := math.Max(0.5, 1+s.growth*float64(d))
	wave := 1 + s.wave*math.Sin(2*math.Pi*float64(d)/30+phase(seed))
	v := s.base * trend * wave * (1 + s.noise*noise(seed, d))
	if isWeekend(t) && s.weekend > 0 {
		v *= s.weekend
	}
	if s.spikeEvery > 0 && unit(seed+100, d) < s.spikeEvery {
		v *= 1 + s.spikeMult*unit(seed+200, d)
	}
	return v
}

// serviceCosts returns every mockServices entry's cost on day t, in
// mockServices order, with Tax derived from the rest.
func serviceCosts(t time.Time) []float64 {
	costs := make([]float64, len(mockServices))
	subtotal := 0.0
	taxIdx := -1
	for i, s := range mockServices {
		if s.name == taxService {
			taxIdx = i
			continue
		}
		costs[i] = s.dailyCost(i, t)
		subtotal += costs[i]
	}
	if taxIdx >= 0 {
		costs[taxIdx] = subtotal * taxRate
	}
	return costs
}

// sharesFor returns each label's share of spend on day t for a non-SERVICE
// dimension. Shares wobble slowly around baseShares but are renormalized to
// sum to exactly 1, so groupings always reconcile with the total.
func sharesFor(dimension string, t time.Time) []float64 {
	base, ok := baseShares[dimension]
	if !ok {
		n := len(labelsFor(dimension))
		base = make([]float64, n)
		for i := range base {
			base[i] = 1 / float64(n)
		}
	}
	d := float64(dayNumber(t))
	out := make([]float64, len(base))
	sum := 0.0
	for i, share := range base {
		out[i] = share * (1 + 0.06*math.Sin(2*math.Pi*d/45+float64(i)*2.1))
		sum += out[i]
	}
	for i := range out {
		out[i] /= sum
	}
	return out
}

// signal builds a smooth percentage-style series: a center value, a slow
// sine wave of the given period (days), a weekend shift and small noise.
func signal(seed int, t time.Time, center, slowAmp, periodDays, weekendDelta, noiseAmp float64) float64 {
	d := dayNumber(t)
	v := center + slowAmp*math.Sin(2*math.Pi*float64(d)/periodDays+phase(seed))
	if isWeekend(t) {
		v += weekendDelta
	}
	return v + noiseAmp*noise(seed+300, d)
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
