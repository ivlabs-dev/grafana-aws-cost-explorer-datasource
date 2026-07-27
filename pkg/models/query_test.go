package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestQueryValidation(t *testing.T) {
	tests := []struct {
		name    string
		query   Query
		wantErr bool
	}{
		{
			name: "valid two groups",
			query: Query{
				Version: QueryVersion, Metric: "AmortizedCost",
				Granularity: GranularityMonthly,
				GroupBy:     []string{"SERVICE", "LINKED_ACCOUNT"},
				Format:      FormatTable,
			},
		},
		{
			name: "too many groups",
			query: Query{
				Version: QueryVersion, Metric: "UnblendedCost",
				Granularity: GranularityDaily,
				GroupBy:     []string{"SERVICE", "LINKED_ACCOUNT", "REGION"},
				Format:      FormatTimeSeries,
			},
			wantErr: true,
		},
		{
			name: "duplicate group",
			query: Query{
				Version: QueryVersion, Metric: "UnblendedCost",
				Granularity: GranularityDaily,
				GroupBy:     []string{"SERVICE", "SERVICE"},
				Format:      FormatTimeSeries,
			},
			wantErr: true,
		},
		{
			name: "partial tag filter",
			query: Query{
				Version: QueryVersion, Metric: "UnblendedCost",
				Granularity: GranularityDaily,
				Filter:      QueryFilter{TagKey: "Environment"},
				Format:      FormatTimeSeries,
			},
			wantErr: true,
		},
		{
			name: "invalid top N",
			query: Query{
				Version: QueryVersion, Metric: "UnblendedCost",
				Granularity: GranularityDaily,
				GroupBy:     []string{"SERVICE"},
				Format:      FormatTimeSeries,
				TopN:        3,
				RangeMode:   RangeModeDashboard,
			},
			wantErr: true,
		},
		{
			name: "top N without grouping",
			query: Query{
				Version: QueryVersion, Metric: "UnblendedCost",
				Granularity: GranularityDaily,
				Format:      FormatTimeSeries,
				TopN:        5,
				RangeMode:   RangeModeDashboard,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.query.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestLegacyQueryJSONAppliesCompatibleDefaults(t *testing.T) {
	var query Query
	err := json.Unmarshal([]byte(`{
		"version": 1,
		"metric": "UnblendedCost",
		"granularity": "DAILY",
		"groupBy": ["SERVICE"],
		"filter": {},
		"format": "timeSeries"
	}`), &query)
	if err != nil {
		t.Fatal(err)
	}

	query.ApplyDefaults()

	if query.IncludeIncompletePeriod != DefaultIncludeIncompletePeriod {
		t.Fatalf("includeIncompletePeriod = %v, want %v", query.IncludeIncompletePeriod, DefaultIncludeIncompletePeriod)
	}
	if query.TopN != DefaultTopN {
		t.Fatalf("topN = %d, want %d", query.TopN, DefaultTopN)
	}
	if !query.IncludeOtherValue() {
		t.Fatal("includeOther = false, want true")
	}
	if !query.AlignMonthlyToCalendarValue() {
		t.Fatal("alignMonthlyToCalendar = false, want true")
	}
	if query.RangeMode != RangeModeDashboard {
		t.Fatalf("rangeMode = %q, want %q", query.RangeMode, RangeModeDashboard)
	}
	if err := query.Validate(); err != nil {
		t.Fatalf("legacy query should remain valid: %v", err)
	}
}

func TestExplicitFalseQueryOptionsSurviveDefaults(t *testing.T) {
	disabled := false
	query := Query{
		IncludeOther:           &disabled,
		AlignMonthlyToCalendar: &disabled,
	}

	query.ApplyDefaults()

	if query.IncludeOtherValue() {
		t.Fatal("explicit includeOther=false was overwritten")
	}
	if query.AlignMonthlyToCalendarValue() {
		t.Fatal("explicit alignMonthlyToCalendar=false was overwritten")
	}
}

func TestCostExplorerDateRangeRoundsNonMidnightEndUp(t *testing.T) {
	from := time.Date(2026, 7, 1, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	to := time.Date(2026, 7, 3, 15, 45, 0, 0, time.FixedZone("CEST", 2*60*60))

	got, err := CostExplorerDateRange(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if got.Start != "2026-07-01" || got.End != "2026-07-04" {
		t.Fatalf("got %+v, want start 2026-07-01 and exclusive end 2026-07-04", got)
	}
}

func TestCostExplorerDateRangeKeepsMidnightEndExclusive(t *testing.T) {
	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)

	got, err := CostExplorerDateRange(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if got.End != "2026-07-03" {
		t.Fatalf("got exclusive end %s, want 2026-07-03", got.End)
	}
}
