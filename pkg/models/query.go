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
)

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

type Query struct {
	Version                 int         `json:"version"`
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
}

func (q Query) Validate() error {
	if q.Version != QueryVersion {
		return fmt.Errorf("query model version %d is unsupported; expected version %d", q.Version, QueryVersion)
	}
	if _, ok := validMetrics[q.Metric]; !ok {
		return fmt.Errorf("metric %q is unsupported", q.Metric)
	}
	if q.Granularity != GranularityDaily && q.Granularity != GranularityMonthly {
		return fmt.Errorf("granularity must be DAILY or MONTHLY")
	}
	if q.Format != FormatTimeSeries && q.Format != FormatTable {
		return fmt.Errorf("result format must be timeSeries or table")
	}
	if len(q.GroupBy) > 2 {
		return fmt.Errorf("AWS Cost Explorer supports at most two group-by dimensions")
	}
	switch q.TopN {
	case 0, 5, 10, 20:
	default:
		return fmt.Errorf("top N must be one of 0, 5, 10, or 20")
	}
	if q.TopN > 0 && len(q.GroupBy) == 0 {
		return fmt.Errorf("top N requires at least one group-by dimension")
	}
	switch q.RangeMode {
	case "", RangeModeDashboard, RangeModeMonthToDate, RangeModePreviousEquivalentPeriod:
	default:
		return fmt.Errorf("query range mode %q is unsupported", q.RangeMode)
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
