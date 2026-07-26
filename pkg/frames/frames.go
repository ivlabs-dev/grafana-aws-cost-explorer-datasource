package frames

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

func Convert(query models.Query, output *awscostexplorer.GetCostAndUsageOutput) (data.Frames, error) {
	if output == nil {
		return nil, fmt.Errorf("response from AWS Cost Explorer is nil")
	}
	if query.Format == models.FormatTable {
		frame, err := tableFrame(query, output)
		if err != nil {
			return nil, err
		}
		return data.Frames{frame}, nil
	}
	return timeSeriesFrames(query, output)
}

type series struct {
	name       string
	labels     data.Labels
	times      []time.Time
	values     []float64
	unit       string
	mixedUnits bool
}

func timeSeriesFrames(query models.Query, output *awscostexplorer.GetCostAndUsageOutput) (data.Frames, error) {
	byKey := make(map[string]*series)

	for _, period := range output.ResultsByTime {
		timestamp, err := periodStart(period)
		if err != nil {
			return nil, err
		}

		if len(query.GroupBy) == 0 {
			metric, ok := period.Total[query.Metric]
			if !ok {
				continue
			}
			item := byKey["total"]
			if item == nil {
				item = &series{name: query.Metric, labels: data.Labels{}}
				byKey["total"] = item
			}
			if err := appendPoint(item, timestamp, metric); err != nil {
				return nil, err
			}
			continue
		}

		for _, group := range period.Groups {
			metric, ok := group.Metrics[query.Metric]
			if !ok {
				continue
			}
			key := strings.Join(group.Keys, "\x00")
			item := byKey[key]
			if item == nil {
				labels := make(data.Labels, len(query.GroupBy))
				parts := make([]string, 0, len(query.GroupBy))
				for index, dimension := range query.GroupBy {
					value := ""
					if index < len(group.Keys) {
						value = group.Keys[index]
					}
					labels[dimension] = value
					parts = append(parts, fmt.Sprintf("%s=%s", dimension, value))
				}
				item = &series{
					name:   strings.Join(parts, ", "),
					labels: labels,
				}
				byKey[key] = item
			}
			if err := appendPoint(item, timestamp, metric); err != nil {
				return nil, err
			}
		}
	}

	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	result := make(data.Frames, 0, max(1, len(keys)))
	for _, key := range keys {
		item := byKey[key]
		sortSeries(item)
		valueField := data.NewField(query.Metric, item.labels, item.values)
		valueField.Config = &data.FieldConfig{
			DisplayNameFromDS: item.name,
			Unit:              grafanaUnit(item.unit),
		}
		result = append(result, data.NewFrame(
			item.name,
			data.NewField("Time", nil, item.times),
			valueField,
		))
	}

	if len(result) == 0 {
		valueField := data.NewField(query.Metric, data.Labels{}, []float64{})
		result = append(result, data.NewFrame(
			query.Metric,
			data.NewField("Time", nil, []time.Time{}),
			valueField,
		))
	}

	return result, nil
}

func tableFrame(query models.Query, output *awscostexplorer.GetCostAndUsageOutput) (*data.Frame, error) {
	times := make([]time.Time, 0)
	groupValues := make([][]string, len(query.GroupBy))
	metrics := make([]string, 0)
	amounts := make([]float64, 0)
	units := make([]string, 0)

	appendRow := func(timestamp time.Time, keys []string, metric types.MetricValue) error {
		amount, err := metricAmount(metric)
		if err != nil {
			return err
		}
		times = append(times, timestamp)
		for index := range groupValues {
			value := ""
			if index < len(keys) {
				value = keys[index]
			}
			groupValues[index] = append(groupValues[index], value)
		}
		metrics = append(metrics, query.Metric)
		amounts = append(amounts, amount)
		units = append(units, stringValue(metric.Unit))
		return nil
	}

	for _, period := range output.ResultsByTime {
		timestamp, err := periodStart(period)
		if err != nil {
			return nil, err
		}
		if len(query.GroupBy) == 0 {
			if metric, ok := period.Total[query.Metric]; ok {
				if err := appendRow(timestamp, nil, metric); err != nil {
					return nil, err
				}
			}
			continue
		}
		for _, group := range period.Groups {
			if metric, ok := group.Metrics[query.Metric]; ok {
				if err := appendRow(timestamp, group.Keys, metric); err != nil {
					return nil, err
				}
			}
		}
	}

	fields := []*data.Field{data.NewField("Time period", nil, times)}
	for index, dimension := range query.GroupBy {
		fields = append(fields, data.NewField(dimension, nil, groupValues[index]))
	}
	fields = append(fields,
		data.NewField("Metric", nil, metrics),
		data.NewField("Amount", nil, amounts),
		data.NewField("Unit", nil, units),
	)

	return data.NewFrame("AWS Cost Explorer", fields...), nil
}

func appendPoint(item *series, timestamp time.Time, metric types.MetricValue) error {
	amount, err := metricAmount(metric)
	if err != nil {
		return err
	}
	item.times = append(item.times, timestamp)
	item.values = append(item.values, amount)
	unit := stringValue(metric.Unit)
	if item.mixedUnits {
		return nil
	}
	if item.unit == "" {
		item.unit = unit
	} else if unit != item.unit {
		item.unit = ""
		item.mixedUnits = true
	}
	return nil
}

func metricAmount(metric types.MetricValue) (float64, error) {
	if metric.Amount == nil {
		return 0, fmt.Errorf("metric amount from AWS Cost Explorer is missing")
	}
	value, err := strconv.ParseFloat(*metric.Amount, 64)
	if err != nil {
		return 0, fmt.Errorf("parse Cost Explorer amount %q: %w", *metric.Amount, err)
	}
	return value, nil
}

func periodStart(period types.ResultByTime) (time.Time, error) {
	if period.TimePeriod == nil || period.TimePeriod.Start == nil {
		return time.Time{}, fmt.Errorf("response time period from AWS Cost Explorer is missing a start date")
	}
	value, err := time.Parse(time.DateOnly, *period.TimePeriod.Start)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse Cost Explorer period start %q: %w", *period.TimePeriod.Start, err)
	}
	return value.UTC(), nil
}

func sortSeries(item *series) {
	indexes := make([]int, len(item.times))
	for index := range indexes {
		indexes[index] = index
	}
	sort.SliceStable(indexes, func(left, right int) bool {
		return item.times[indexes[left]].Before(item.times[indexes[right]])
	})

	sortedTimes := make([]time.Time, len(item.times))
	sortedValues := make([]float64, len(item.values))
	for target, source := range indexes {
		sortedTimes[target] = item.times[source]
		sortedValues[target] = item.values[source]
	}
	item.times = sortedTimes
	item.values = sortedValues
}

func grafanaUnit(unit string) string {
	if len(unit) == 3 && strings.ToUpper(unit) == unit {
		return "currency" + unit
	}
	return unit
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
