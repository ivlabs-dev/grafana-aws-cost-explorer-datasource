package frames

import (
	"sort"
	"strings"
	"time"

	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

func Convert(query models.Query, output *awscostexplorer.GetCostAndUsageOutput) (data.Frames, error) {
	prepared, err := PrepareResults(query, output)
	if err != nil {
		return nil, err
	}
	if query.Format == models.FormatTable {
		return data.Frames{tableFrame(query, prepared)}, nil
	}
	return timeSeriesFrames(query, prepared), nil
}

func timeSeriesFrames(query models.Query, prepared []PreparedSeries) data.Frames {
	result := make(data.Frames, 0, max(1, len(prepared)))
	for _, item := range prepared {
		times := make([]time.Time, len(item.Points))
		values := make([]float64, len(item.Points))
		for index, point := range item.Points {
			times[index] = point.PeriodStart.UTC()
			values[index] = point.Amount
		}

		valueField := data.NewField(query.Metric, item.Labels, values)
		valueField.Config = &data.FieldConfig{
			DisplayNameFromDS: item.DisplayName,
			Unit:              grafanaUnit(item.Unit),
		}
		result = append(result, data.NewFrame(
			item.DisplayName,
			data.NewField("Time", nil, times),
			valueField,
		))
	}

	if len(result) == 0 {
		valueField := data.NewField(query.Metric, data.Labels{}, []float64{})
		valueField.Config = &data.FieldConfig{DisplayNameFromDS: query.Metric}
		result = append(result, data.NewFrame(
			query.Metric,
			data.NewField("Time", nil, []time.Time{}),
			valueField,
		))
	}

	return result
}

type tableRow struct {
	period      time.Time
	identity    string
	groupValues []string
	amount      float64
	unit        string
}

func tableFrame(query models.Query, prepared []PreparedSeries) *data.Frame {
	rows := make([]tableRow, 0)
	for _, item := range prepared {
		for _, point := range item.Points {
			rows = append(rows, tableRow{
				period:      point.PeriodStart.UTC(),
				identity:    item.Identity,
				groupValues: append([]string(nil), item.GroupValues...),
				amount:      point.Amount,
				unit:        item.Unit,
			})
		}
	}
	sort.SliceStable(rows, func(left, right int) bool {
		if rows[left].period.Equal(rows[right].period) {
			return rows[left].identity < rows[right].identity
		}
		return rows[left].period.Before(rows[right].period)
	})

	periods := make([]string, 0, len(rows))
	groupValues := make([][]string, len(query.GroupBy))
	metrics := make([]string, 0, len(rows))
	amounts := make([]float64, 0, len(rows))
	units := make([]string, 0, len(rows))
	for _, row := range rows {
		periods = append(periods, FormatBillingPeriodLabel(query.Granularity, row.period))
		for index := range groupValues {
			value := ""
			if index < len(row.groupValues) {
				value = row.groupValues[index]
			}
			groupValues[index] = append(groupValues[index], value)
		}
		metrics = append(metrics, query.Metric)
		amounts = append(amounts, row.amount)
		units = append(units, row.unit)
	}

	fields := []*data.Field{data.NewField("Period", nil, periods)}
	for index, dimension := range query.GroupBy {
		fields = append(fields, data.NewField(dimension, nil, groupValues[index]))
	}
	fields = append(fields,
		data.NewField("Metric", nil, metrics),
		data.NewField("Amount", nil, amounts),
		data.NewField("Unit", nil, units),
	)

	return data.NewFrame("AWS Cost Explorer", fields...)
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
