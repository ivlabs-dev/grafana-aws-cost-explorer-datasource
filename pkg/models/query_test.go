package models

import (
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
