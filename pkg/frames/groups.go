package frames

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

const (
	otherGroupLabel = "costexplorer_group"
	unitGroupLabel  = "costexplorer_unit"
)

// PreparedPoint is a frame-compatible value for one AWS billing period.
type PreparedPoint struct {
	PeriodStart time.Time
	Amount      float64
}

// PreparedSeries is a normalized result series shared by the table and time
// series conversion paths. Identity is stable and includes dimensions, values,
// and the metric unit; DisplayName intentionally contains only human-readable
// group values.
type PreparedSeries struct {
	Identity    string
	DisplayName string
	Labels      data.Labels
	GroupValues []string
	Unit        string
	Points      []PreparedPoint
	IsOther     bool
}

type accumulatedSeries struct {
	identity      string
	baseIdentity  string
	displayName   string
	labels        data.Labels
	groupValues   []string
	unit          string
	points        map[time.Time]*big.Rat
	total         *big.Rat
	isOther       bool
	hasMixedUnits bool
}

// PrepareResults normalizes a fully paginated Cost Explorer response. For
// grouped queries it ranks complete-range totals per unit, then optionally
// aggregates excluded groups into one Other series per period and unit.
func PrepareResults(query models.Query, output *awscostexplorer.GetCostAndUsageOutput) ([]PreparedSeries, error) {
	if output == nil {
		return nil, fmt.Errorf("response from AWS Cost Explorer is nil")
	}

	if len(query.GroupBy) == 0 {
		return prepareUngroupedResults(query, output)
	}

	byIdentity := make(map[string]*accumulatedSeries)
	allPeriods := make(map[time.Time]struct{})
	for _, period := range output.ResultsByTime {
		start, err := BillingPeriodStartUTC(period)
		if err != nil {
			return nil, err
		}
		allPeriods[start] = struct{}{}
		for _, group := range period.Groups {
			metric, ok := group.Metrics[query.Metric]
			if !ok {
				continue
			}
			amount, err := exactMetricAmount(metric)
			if err != nil {
				return nil, err
			}
			unit := stringValue(metric.Unit)
			baseIdentity, displayName, labels, values := groupPresentation(query.Metric, query.GroupBy, group.Keys)
			identity := canonicalIdentity(baseIdentity, unit)
			item := byIdentity[identity]
			if item == nil {
				item = &accumulatedSeries{
					identity:     identity,
					baseIdentity: baseIdentity,
					displayName:  displayName,
					labels:       labels,
					groupValues:  values,
					unit:         unit,
					points:       make(map[time.Time]*big.Rat),
					total:        new(big.Rat),
				}
				byIdentity[identity] = item
			}
			addExact(item.points, start, amount)
			item.total.Add(item.total, amount)
		}
	}

	items := make([]*accumulatedSeries, 0, len(byIdentity))
	for _, item := range byIdentity {
		items = append(items, item)
	}
	if query.TopN > 0 {
		items = limitGroups(items, query.TopN, query.IncludeOtherValue(), len(query.GroupBy), allPeriods)
	} else {
		sort.Slice(items, func(left, right int) bool {
			return items[left].identity < items[right].identity
		})
	}
	addUnitLabelsForDuplicateIdentities(items)

	return materializeSeries(items)
}

func prepareUngroupedResults(
	query models.Query,
	output *awscostexplorer.GetCostAndUsageOutput,
) ([]PreparedSeries, error) {
	item := &accumulatedSeries{
		identity:     canonicalIdentity("total", ""),
		baseIdentity: "total",
		displayName:  query.Metric,
		labels:       data.Labels{},
		points:       make(map[time.Time]*big.Rat),
		total:        new(big.Rat),
	}
	hasValue := false
	for _, period := range output.ResultsByTime {
		metric, ok := period.Total[query.Metric]
		if !ok {
			continue
		}
		start, err := BillingPeriodStartUTC(period)
		if err != nil {
			return nil, err
		}
		amount, err := exactMetricAmount(metric)
		if err != nil {
			return nil, err
		}
		unit := stringValue(metric.Unit)
		if !hasValue {
			item.unit = unit
		} else if unit != item.unit {
			item.unit = ""
			item.hasMixedUnits = true
		}
		addExact(item.points, start, amount)
		item.total.Add(item.total, amount)
		hasValue = true
	}
	if !hasValue {
		return []PreparedSeries{}, nil
	}
	return materializeSeries([]*accumulatedSeries{item})
}

func groupPresentation(
	metric string,
	dimensions []string,
	groupValues []string,
) (string, string, data.Labels, []string) {
	if len(dimensions) == 0 {
		return "total", metric, data.Labels{}, nil
	}

	labels := make(data.Labels, len(dimensions))
	values := make([]string, len(dimensions))
	visible := make([]string, 0, len(dimensions))
	identityParts := make([]string, 0, len(dimensions)*2)
	for index, dimension := range dimensions {
		value := ""
		if index < len(groupValues) {
			value = groupValues[index]
		}
		values[index] = value
		labels[dimension] = value
		identityParts = append(identityParts, dimension, value)
		if cleaned := strings.TrimSpace(value); cleaned != "" {
			visible = append(visible, cleaned)
		}
	}

	displayName := strings.Join(visible, " · ")
	if displayName == "" {
		displayName = "(no group value)"
	}
	return canonicalParts(identityParts...), displayName, labels, values
}

func limitGroups(
	items []*accumulatedSeries,
	topN int,
	includeOther bool,
	dimensionCount int,
	allPeriods map[time.Time]struct{},
) []*accumulatedSeries {
	byUnit := make(map[string][]*accumulatedSeries)
	for _, item := range items {
		byUnit[item.unit] = append(byUnit[item.unit], item)
	}
	units := make([]string, 0, len(byUnit))
	for unit := range byUnit {
		units = append(units, unit)
	}
	sort.Strings(units)

	limited := make([]*accumulatedSeries, 0, len(items))
	for _, unit := range units {
		unitItems := byUnit[unit]
		sort.Slice(unitItems, func(left, right int) bool {
			comparison := unitItems[left].total.Cmp(unitItems[right].total)
			if comparison != 0 {
				return comparison > 0
			}
			return unitItems[left].identity < unitItems[right].identity
		})

		keep := min(topN, len(unitItems))
		limited = append(limited, unitItems[:keep]...)
		if !includeOther || keep == len(unitItems) {
			continue
		}

		values := make([]string, dimensionCount)
		if len(values) > 0 {
			values[0] = "Other"
		}
		other := &accumulatedSeries{
			identity:     canonicalIdentity("other", unit),
			baseIdentity: "other",
			displayName:  "Other",
			labels:       data.Labels{otherGroupLabel: "Other"},
			groupValues:  values,
			unit:         unit,
			points:       make(map[time.Time]*big.Rat),
			total:        new(big.Rat),
			isOther:      true,
		}
		for _, item := range unitItems[keep:] {
			other.total.Add(other.total, item.total)
			for period, amount := range item.points {
				addExact(other.points, period, amount)
			}
		}
		for period := range allPeriods {
			if other.points[period] == nil {
				other.points[period] = new(big.Rat)
			}
		}
		limited = append(limited, other)
	}
	return limited
}

func materializeSeries(items []*accumulatedSeries) ([]PreparedSeries, error) {
	result := make([]PreparedSeries, 0, len(items))
	for _, item := range items {
		periods := make([]time.Time, 0, len(item.points))
		for period := range item.points {
			periods = append(periods, period)
		}
		sort.Slice(periods, func(left, right int) bool {
			return periods[left].Before(periods[right])
		})

		points := make([]PreparedPoint, 0, len(periods))
		for _, period := range periods {
			amount, _ := item.points[period].Float64()
			if math.IsInf(amount, 0) || math.IsNaN(amount) {
				return nil, fmt.Errorf("Cost Explorer amount cannot be represented as a Grafana number")
			}
			points = append(points, PreparedPoint{PeriodStart: period, Amount: amount})
		}
		unit := item.unit
		if item.hasMixedUnits {
			unit = ""
		}
		result = append(result, PreparedSeries{
			Identity:    item.identity,
			DisplayName: item.displayName,
			Labels:      item.labels,
			GroupValues: item.groupValues,
			Unit:        unit,
			Points:      points,
			IsOther:     item.isOther,
		})
	}
	return result, nil
}

func addUnitLabelsForDuplicateIdentities(items []*accumulatedSeries) {
	unitsByIdentity := make(map[string]map[string]struct{})
	for _, item := range items {
		if unitsByIdentity[item.baseIdentity] == nil {
			unitsByIdentity[item.baseIdentity] = make(map[string]struct{})
		}
		unitsByIdentity[item.baseIdentity][item.unit] = struct{}{}
	}
	for _, item := range items {
		if len(unitsByIdentity[item.baseIdentity]) > 1 {
			item.labels[unitGroupLabel] = item.unit
		}
	}
}

func exactMetricAmount(metric types.MetricValue) (*big.Rat, error) {
	if metric.Amount == nil {
		return nil, fmt.Errorf("metric amount from AWS Cost Explorer is missing")
	}
	value, ok := new(big.Rat).SetString(*metric.Amount)
	if !ok {
		return nil, fmt.Errorf("parse Cost Explorer amount %q: invalid decimal", *metric.Amount)
	}
	return value, nil
}

func addExact(points map[time.Time]*big.Rat, period time.Time, amount *big.Rat) {
	existing := points[period]
	if existing == nil {
		points[period] = new(big.Rat).Set(amount)
		return
	}
	existing.Add(existing, amount)
}

func canonicalIdentity(baseIdentity, unit string) string {
	return canonicalParts(baseIdentity, unit)
}

func canonicalParts(parts ...string) string {
	var result strings.Builder
	for _, part := range parts {
		fmt.Fprintf(&result, "%d:%s;", len(part), part)
	}
	return result.String()
}
