package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	QueryVersion = 1

	GranularityDaily   = "DAILY"
	GranularityMonthly = "MONTHLY"

	FormatTimeSeries = "timeSeries"
	FormatTable      = "table"

	RangeModeDashboard                = "dashboard"
	RangeModeMonthToDate              = "monthToDate"
	RangeModePreviousEquivalentPeriod = "previousEquivalentPeriod"

	DefaultIncludeIncompletePeriod = false
	DefaultTopN                    = 0
	DefaultIncludeOther            = true
	DefaultAlignMonthlyToCalendar  = true

	QueryTypeCostAndUsage            = "costAndUsage"
	QueryTypeReservationUtilization  = "reservationUtilization"
	QueryTypeSavingsPlansUtilization = "savingsPlansUtilization"

	DefaultUtilizationMetric = "UtilizationPercentage"

	// DefaultReservationService matches AWS's own undocumented-in-the-UI
	// default: GetReservationUtilization silently assumes EC2 when the
	// SERVICE filter is omitted. This plugin makes that default explicit
	// and visible instead of leaving it silent.
	DefaultReservationService = "Amazon Elastic Compute Cloud - Compute"
)

// ReservationEligibleServices are the only SERVICE filter values AWS accepts
// for GetReservationUtilization (one per request, not combinable). Leaving
// this filter empty doesn't mean "all reservations" -- AWS silently
// interprets it as DefaultReservationService, which can make an account with
// real, active reservations in another one of these services (RDS being the
// most common case) look like it has no reservations at all. This plugin
// requires an explicit choice from this list rather than allowing that trap.
var ReservationEligibleServices = []string{
	DefaultReservationService,
	"Amazon Relational Database Service",
	"Amazon ElastiCache",
	"Amazon Redshift",
	"Amazon Elasticsearch Service",
}

// ReservationUtilizationMetrics is the curated subset of
// types.ReservationAggregates fields this plugin exposes as a selectable
// time-series metric or a table column. AWS returns ~17 fields; this list is
// an intentional, documented scope boundary rather than the full set.
var ReservationUtilizationMetrics = []string{
	"UtilizationPercentage",
	"NetRISavings",
	"OnDemandCostOfRIHoursUsed",
	"TotalAmortizedFee",
	"UnusedHours",
	"RealizedSavings",
	"UnrealizedSavings",
}

// SavingsPlansUtilizationMetrics is the curated subset of Savings Plans
// utilization/savings/amortized-commitment fields this plugin exposes.
var SavingsPlansUtilizationMetrics = []string{
	"UtilizationPercentage",
	"UsedCommitment",
	"UnusedCommitment",
	"TotalCommitment",
	"NetSavings",
	"OnDemandCostEquivalent",
	"TotalAmortizedCommitment",
}

var validMetrics = map[string]struct{}{
	"UnblendedCost":    {},
	"BlendedCost":      {},
	"AmortizedCost":    {},
	"NetAmortizedCost": {},
	"NetUnblendedCost": {},
	"UsageQuantity":    {},
}

var validGroupings = map[string]struct{}{
	"SERVICE":           {},
	"LINKED_ACCOUNT":    {},
	"REGION":            {},
	"INSTANCE_TYPE":     {},
	"PURCHASE_TYPE":     {},
	"USAGE_TYPE":        {},
	"OPERATION":         {},
	"AVAILABILITY_ZONE": {},
}

func isReservationEligibleService(service string) bool {
	for _, eligible := range ReservationEligibleServices {
		if service == eligible {
			return true
		}
	}
	return false
}

type Query struct {
	Version                 int         `json:"version"`
	QueryType               string      `json:"queryType,omitempty"`
	Metric                  string      `json:"metric"`
	Granularity             string      `json:"granularity"`
	GroupBy                 []string    `json:"groupBy,omitempty"`
	Filter                  QueryFilter `json:"filter,omitempty"`
	Format                  string      `json:"format"`
	IncludeIncompletePeriod bool        `json:"includeIncompletePeriod"`
	TopN                    int         `json:"topN"`
	IncludeOther            *bool       `json:"includeOther,omitempty"`
	AlignMonthlyToCalendar  *bool       `json:"alignMonthlyToCalendar,omitempty"`
	RangeMode               string      `json:"rangeMode"`
	GroupBySubscription     bool        `json:"groupBySubscription,omitempty"`
	UtilizationMetric       string      `json:"utilizationMetric,omitempty"`
}

type QueryFilter struct {
	Service       string `json:"service,omitempty"`
	LinkedAccount string `json:"linkedAccount,omitempty"`
	Region        string `json:"region,omitempty"`
	TagKey        string `json:"tagKey,omitempty"`
	TagValue      string `json:"tagValue,omitempty"`
}

type DateRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

func (q *Query) ApplyDefaults() {
	if q.Version == 0 {
		q.Version = QueryVersion
	}
	if q.QueryType == "" {
		q.QueryType = QueryTypeCostAndUsage
	}
	if q.Metric == "" {
		q.Metric = "UnblendedCost"
	}
	if q.Granularity == "" {
		q.Granularity = GranularityDaily
	}
	if q.Format == "" {
		q.Format = FormatTimeSeries
	}
	if q.GroupBy == nil {
		q.GroupBy = []string{}
	}
	if q.IncludeOther == nil {
		q.IncludeOther = boolValue(DefaultIncludeOther)
	}
	if q.AlignMonthlyToCalendar == nil {
		q.AlignMonthlyToCalendar = boolValue(DefaultAlignMonthlyToCalendar)
	}
	if q.RangeMode == "" {
		q.RangeMode = RangeModeDashboard
	}
	if q.UtilizationMetric == "" {
		q.UtilizationMetric = DefaultUtilizationMetric
	}
	if q.QueryType == QueryTypeReservationUtilization && q.Filter.Service == "" {
		q.Filter.Service = DefaultReservationService
	}
}

// IsUtilizationQuery reports whether this query targets one of the
// Reservation/Savings Plans utilization APIs rather than GetCostAndUsage.
func (q Query) IsUtilizationQuery() bool {
	switch q.QueryType {
	case QueryTypeReservationUtilization, QueryTypeSavingsPlansUtilization:
		return true
	default:
		return false
	}
}

func (q Query) Validate() error {
	if q.Version != QueryVersion {
		return fmt.Errorf("query model version %d is unsupported; expected version %d", q.Version, QueryVersion)
	}
	switch q.QueryType {
	case "", QueryTypeCostAndUsage, QueryTypeReservationUtilization, QueryTypeSavingsPlansUtilization:
	default:
		return fmt.Errorf("query type %q is unsupported", q.QueryType)
	}
	if q.Granularity != GranularityDaily && q.Granularity != GranularityMonthly {
		return fmt.Errorf("granularity must be DAILY or MONTHLY")
	}
	if q.Format != FormatTimeSeries && q.Format != FormatTable {
		return fmt.Errorf("result format must be timeSeries or table")
	}
	switch q.RangeMode {
	case "", RangeModeDashboard, RangeModeMonthToDate, RangeModePreviousEquivalentPeriod:
	default:
		return fmt.Errorf("query range mode %q is unsupported", q.RangeMode)
	}

	if q.IsUtilizationQuery() {
		return q.validateUtilization()
	}

	if _, ok := validMetrics[q.Metric]; !ok {
		return fmt.Errorf("metric %q is unsupported", q.Metric)
	}
	if len(q.GroupBy) > 2 {
		return fmt.Errorf("AWS Cost Explorer supports at most two group-by dimensions")
	}
	switch q.TopN {
	case 0, 1, 5, 10, 20:
	default:
		return fmt.Errorf("top N must be one of 0, 1, 5, 10, or 20")
	}
	if q.TopN > 0 && len(q.GroupBy) == 0 {
		return fmt.Errorf("top N requires at least one group-by dimension")
	}

	seen := make(map[string]struct{}, len(q.GroupBy))
	for _, group := range q.GroupBy {
		if _, ok := validGroupings[group]; !ok {
			return fmt.Errorf("group-by dimension %q is unsupported", group)
		}
		if _, duplicate := seen[group]; duplicate {
			return fmt.Errorf("group-by dimension %q is duplicated", group)
		}
		seen[group] = struct{}{}
	}

	hasTagKey := strings.TrimSpace(q.Filter.TagKey) != ""
	hasTagValue := strings.TrimSpace(q.Filter.TagValue) != ""
	if hasTagKey != hasTagValue {
		return fmt.Errorf("cost allocation tag filtering requires both a tag key and a tag value")
	}

	return nil
}

// validateUtilization applies the narrower rules for
// QueryTypeReservationUtilization/QueryTypeSavingsPlansUtilization: neither
// AWS API supports cost allocation tag filtering, Savings Plans utilization
// doesn't support a SERVICE filter or any grouping, and the selected
// utilization metric must be one this plugin actually knows how to render.
func (q Query) validateUtilization() error {
	if strings.TrimSpace(q.Filter.TagKey) != "" || strings.TrimSpace(q.Filter.TagValue) != "" {
		return fmt.Errorf("cost allocation tag filtering is not supported for %q queries", q.QueryType)
	}

	var allowedMetrics []string
	switch q.QueryType {
	case QueryTypeReservationUtilization:
		if !isReservationEligibleService(q.Filter.Service) {
			return fmt.Errorf(
				"AWS service %q is not valid for Reservation utilization; it must be one of %v "+
					"(AWS silently assumes EC2 if left unset, which can hide real reservations in another service)",
				q.Filter.Service, ReservationEligibleServices,
			)
		}
		allowedMetrics = ReservationUtilizationMetrics
	case QueryTypeSavingsPlansUtilization:
		if strings.TrimSpace(q.Filter.Service) != "" {
			return fmt.Errorf("the service filter is not supported for Savings Plans utilization queries")
		}
		if q.GroupBySubscription {
			return fmt.Errorf("grouping by subscription is not supported for Savings Plans utilization queries")
		}
		allowedMetrics = SavingsPlansUtilizationMetrics
	}

	for _, metric := range allowedMetrics {
		if metric == q.UtilizationMetric {
			return nil
		}
	}
	return fmt.Errorf("utilization metric %q is unsupported for %q queries", q.UtilizationMetric, q.QueryType)
}

func (q Query) IncludeOtherValue() bool {
	return q.IncludeOther == nil || *q.IncludeOther
}

func (q Query) AlignMonthlyToCalendarValue() bool {
	return q.AlignMonthlyToCalendar == nil || *q.AlignMonthlyToCalendar
}

// CostExplorerDateRange converts Grafana's exact timestamp interval to Cost
// Explorer dates. AWS treats Start as inclusive and End as exclusive.
func CostExplorerDateRange(from, to time.Time) (DateRange, error) {
	if !to.After(from) {
		return DateRange{}, fmt.Errorf("dashboard time range end must be after start")
	}

	start := utcDay(from)
	end := utcDay(to)
	if !to.UTC().Equal(end) {
		end = end.AddDate(0, 0, 1)
	}
	if !end.After(start) {
		end = start.AddDate(0, 0, 1)
	}

	return DateRange{
		Start: start.Format(time.DateOnly),
		End:   end.Format(time.DateOnly),
	}, nil
}

func utcDay(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func boolValue(value bool) *bool {
	return &value
}
