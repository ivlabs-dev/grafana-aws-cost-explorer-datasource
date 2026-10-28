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
			name: "internal top one",
			query: Query{
				Version: QueryVersion, Metric: "UnblendedCost",
				Granularity: GranularityDaily,
				GroupBy:     []string{"SERVICE"},
				Format:      FormatTimeSeries,
				TopN:        1,
				RangeMode:   RangeModeDashboard,
			},
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

func TestUtilizationQueryValidation(t *testing.T) {
	base := func() Query {
		q := Query{Version: QueryVersion, QueryType: QueryTypeReservationUtilization, Granularity: GranularityDaily, Format: FormatTimeSeries}
		q.ApplyDefaults()
		return q
	}
	savingsPlansBase := func() Query {
		q := Query{Version: QueryVersion, QueryType: QueryTypeSavingsPlansUtilization, Granularity: GranularityDaily, Format: FormatTimeSeries}
		q.ApplyDefaults()
		return q
	}

	tests := []struct {
		name    string
		query   Query
		wantErr bool
	}{
		{
			name:  "valid reservation utilization",
			query: base(),
		},
		{
			name:  "valid savings plans utilization",
			query: savingsPlansBase(),
		},
		{
			name: "reservation utilization rejects tag filter",
			query: func() Query {
				q := base()
				q.Filter = QueryFilter{TagKey: "Environment", TagValue: "prod"}
				return q
			}(),
			wantErr: true,
		},
		{
			name: "savings plans utilization rejects service filter",
			query: func() Query {
				q := savingsPlansBase()
				q.Filter = QueryFilter{Service: "Amazon Elastic Compute Cloud - Compute"}
				return q
			}(),
			wantErr: true,
		},
		{
			name: "savings plans utilization rejects group by subscription",
			query: func() Query {
				q := savingsPlansBase()
				q.GroupBySubscription = true
				return q
			}(),
			wantErr: true,
		},
		{
			name: "reservation utilization rejects an unknown utilization metric",
			query: func() Query {
				q := base()
				q.UtilizationMetric = "NotARealMetric"
				return q
			}(),
			wantErr: true,
		},
		{
			name: "reservation utilization rejects an invalid service",
			query: func() Query {
				q := base()
				q.Filter.Service = "Amazon Simple Storage Service"
				return q
			}(),
			wantErr: true,
		},
		{
			name: "reservation utilization rejects an empty service",
			query: func() Query {
				q := base()
				q.Filter.Service = ""
				return q
			}(),
			wantErr: true,
		},
		{
			name: "savings plans utilization rejects a reservation-only metric",
			query: func() Query {
				q := savingsPlansBase()
				q.UtilizationMetric = "NetRISavings"
				return q
			}(),
			wantErr: true,
		},
		{
			name: "reservation utilization accepts grouping by subscription",
			query: func() Query {
				q := base()
				q.GroupBySubscription = true
				return q
			}(),
		},
		{
			name: "reservation utilization accepts an RDS service filter",
			query: func() Query {
				q := base()
				q.Filter.Service = "Amazon Relational Database Service"
				return q
			}(),
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

func TestCoverageQueryValidation(t *testing.T) {
	base := func() Query {
		q := Query{Version: QueryVersion, QueryType: QueryTypeReservationCoverage, Granularity: GranularityDaily, Format: FormatTimeSeries}
		q.ApplyDefaults()
		return q
	}
	savingsPlansBase := func() Query {
		q := Query{Version: QueryVersion, QueryType: QueryTypeSavingsPlansCoverage, Granularity: GranularityDaily, Format: FormatTimeSeries}
		q.ApplyDefaults()
		return q
	}

	tests := []struct {
		name    string
		query   Query
		wantErr bool
	}{
		{name: "valid reservation coverage", query: base()},
		{name: "valid savings plans coverage", query: savingsPlansBase()},
		{
			name: "reservation coverage defaults to EC2 and CoverageHoursPercentage",
			query: func() Query {
				q := base()
				if q.Filter.Service != DefaultReservationService {
					t.Fatalf("service default = %q, want %q", q.Filter.Service, DefaultReservationService)
				}
				if q.UtilizationMetric != "CoverageHoursPercentage" {
					t.Fatalf("utilizationMetric default = %q, want CoverageHoursPercentage", q.UtilizationMetric)
				}
				return q
			}(),
		},
		{
			name: "savings plans coverage defaults to CoveragePercentage",
			query: func() Query {
				q := savingsPlansBase()
				if q.UtilizationMetric != "CoveragePercentage" {
					t.Fatalf("utilizationMetric default = %q, want CoveragePercentage", q.UtilizationMetric)
				}
				return q
			}(),
		},
		{
			name: "reservation coverage rejects tag filter",
			query: func() Query {
				q := base()
				q.Filter.TagKey = "Environment"
				q.Filter.TagValue = "prod"
				return q
			}(),
			wantErr: true,
		},
		{
			name: "reservation coverage rejects an invalid service",
			query: func() Query {
				q := base()
				q.Filter.Service = "Amazon Simple Storage Service"
				return q
			}(),
			wantErr: true,
		},
		{
			name: "reservation coverage rejects OpenSearch (valid for Utilization, not Coverage)",
			query: func() Query {
				q := base()
				q.Filter.Service = "Amazon Elasticsearch Service"
				return q
			}(),
			wantErr: true,
		},
		{
			name: "reservation coverage accepts an RDS service filter",
			query: func() Query {
				q := base()
				q.Filter.Service = "Amazon Relational Database Service"
				return q
			}(),
		},
		{
			name: "savings plans coverage rejects a service filter",
			query: func() Query {
				q := savingsPlansBase()
				q.Filter.Service = "Amazon Elastic Compute Cloud - Compute"
				return q
			}(),
			wantErr: true,
		},
		{
			name: "reservation coverage rejects grouping by subscription",
			query: func() Query {
				q := base()
				q.GroupBySubscription = true
				return q
			}(),
			wantErr: true,
		},
		{
			name: "reservation coverage rejects a utilization-only metric",
			query: func() Query {
				q := base()
				q.UtilizationMetric = "UtilizationPercentage"
				return q
			}(),
			wantErr: true,
		},
		{
			name: "savings plans coverage rejects a reservation-only metric",
			query: func() Query {
				q := savingsPlansBase()
				q.UtilizationMetric = "CoverageHoursPercentage"
				return q
			}(),
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

func TestInventoryQueryValidation(t *testing.T) {
	base := func(queryType string) Query {
		q := Query{Version: QueryVersion, QueryType: queryType}
		q.ApplyDefaults()
		return q
	}

	tests := []struct {
		name    string
		query   Query
		wantErr bool
	}{
		{name: "valid reservation inventory", query: base(QueryTypeReservationInventory)},
		{name: "valid savings plans inventory", query: base(QueryTypeSavingsPlansInventory)},
		{
			name: "reservation inventory rejects a filter",
			query: func() Query {
				q := base(QueryTypeReservationInventory)
				q.Filter = QueryFilter{Service: "Amazon Elastic Compute Cloud - Compute"}
				return q
			}(),
			wantErr: true,
		},
		{
			name: "savings plans inventory rejects group-by",
			query: func() Query {
				q := base(QueryTypeSavingsPlansInventory)
				q.GroupBy = []string{"SERVICE"}
				return q
			}(),
			wantErr: true,
		},
		{
			name: "reservation inventory rejects top N",
			query: func() Query {
				q := base(QueryTypeReservationInventory)
				q.TopN = 5
				return q
			}(),
			wantErr: true,
		},
		{
			name: "reservation inventory rejects group by subscription",
			query: func() Query {
				q := base(QueryTypeReservationInventory)
				q.GroupBySubscription = true
				return q
			}(),
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

func TestIsInventoryQuery(t *testing.T) {
	for _, queryType := range []string{QueryTypeReservationInventory, QueryTypeSavingsPlansInventory} {
		if !(Query{QueryType: queryType}).IsInventoryQuery() {
			t.Fatalf("IsInventoryQuery() = false for %q, want true", queryType)
		}
	}
	for _, queryType := range []string{QueryTypeCostAndUsage, QueryTypeReservationUtilization, QueryTypeSavingsPlansUtilization, ""} {
		if (Query{QueryType: queryType}).IsInventoryQuery() {
			t.Fatalf("IsInventoryQuery() = true for %q, want false", queryType)
		}
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

	if query.QueryType != QueryTypeCostAndUsage {
		t.Fatalf("queryType = %q, want %q for a query saved before this field existed", query.QueryType, QueryTypeCostAndUsage)
	}
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
