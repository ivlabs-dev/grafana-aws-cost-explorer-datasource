package frames

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

func reservationCoverageQuery(format string) models.Query {
	q := models.Query{
		QueryType:   models.QueryTypeReservationCoverage,
		Granularity: models.GranularityDaily,
		Format:      format,
	}
	q.ApplyDefaults()
	return q
}

func savingsPlansCoverageQuery(format string) models.Query {
	q := models.Query{
		QueryType:   models.QueryTypeSavingsPlansCoverage,
		Granularity: models.GranularityDaily,
		Format:      format,
	}
	q.ApplyDefaults()
	return q
}

func TestConvertReservationCoverageTable(t *testing.T) {
	query := reservationCoverageQuery(models.FormatTable)
	output := &awscostexplorer.GetReservationCoverageOutput{
		CoveragesByTime: []types.CoverageByTime{{
			TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
			Total: &types.Coverage{
				CoverageHours: &types.CoverageHours{
					CoverageHoursPercentage: aws.String("40%"),
					OnDemandHours:           aws.String("40"),
					ReservedHours:           aws.String("40"),
					TotalRunningHours:       aws.String("80"),
				},
				CoverageCost: &types.CoverageCost{OnDemandCost: aws.String("12.5")},
			},
		}},
	}

	result, err := ConvertReservationCoverage(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Rows() != 1 {
		t.Fatalf("frames = %d, rows = %d", len(result), result[0].Rows())
	}

	coverageField := fieldByName(t, result[0], "CoverageHoursPercentage")
	if got := coverageField.At(0).(float64); got != 40 {
		t.Fatalf("CoverageHoursPercentage = %v, want 40 (the trailing %% should be stripped)", got)
	}
	if got := coverageField.Config.Unit; got != "percent" {
		t.Fatalf("unit = %q, want percent", got)
	}

	hoursField := fieldByName(t, result[0], "OnDemandHours")
	if got := hoursField.Config.Unit; got != "h" {
		t.Fatalf("unit = %q, want h", got)
	}

	costField := fieldByName(t, result[0], "OnDemandCost")
	if got := costField.At(0).(float64); got != 12.5 {
		t.Fatalf("OnDemandCost = %v, want 12.5", got)
	}
	if got := costField.Config.Unit; got != "currencyUSD" {
		t.Fatalf("unit = %q, want currencyUSD", got)
	}
}

func TestConvertReservationCoverageTimeSeries(t *testing.T) {
	query := reservationCoverageQuery(models.FormatTimeSeries)
	query.UtilizationMetric = "CoverageHoursPercentage"
	output := &awscostexplorer.GetReservationCoverageOutput{
		CoveragesByTime: []types.CoverageByTime{
			{
				TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
				Total:      &types.Coverage{CoverageHours: &types.CoverageHours{CoverageHoursPercentage: aws.String("40%")}},
			},
			{
				TimePeriod: &types.DateInterval{Start: aws.String("2026-07-02")},
				Total:      &types.Coverage{CoverageHours: &types.CoverageHours{CoverageHoursPercentage: aws.String("55%")}},
			},
		},
	}

	result, err := ConvertReservationCoverage(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("frames = %d, want 1 (no grouping)", len(result))
	}
	if result[0].Rows() != 2 {
		t.Fatalf("rows = %d, want 2", result[0].Rows())
	}
}

func TestConvertSavingsPlansCoverageTable(t *testing.T) {
	query := savingsPlansCoverageQuery(models.FormatTable)
	output := &awscostexplorer.GetSavingsPlansCoverageOutput{
		SavingsPlansCoverages: []types.SavingsPlansCoverage{{
			TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
			Coverage: &types.SavingsPlansCoverageData{
				CoveragePercentage:         aws.String("52%"),
				SpendCoveredBySavingsPlans: aws.String("100.00"),
				OnDemandCost:               aws.String("92.34"),
				TotalCost:                  aws.String("192.34"),
			},
		}},
	}

	result, err := ConvertSavingsPlansCoverage(query, output)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Rows() != 1 {
		t.Fatalf("frames = %d, rows = %d", len(result), result[0].Rows())
	}

	coverageField := fieldByName(t, result[0], "CoveragePercentage")
	if got := coverageField.At(0).(float64); got != 52 {
		t.Fatalf("CoveragePercentage = %v, want 52 (the trailing %% should be stripped)", got)
	}

	onDemandField := fieldByName(t, result[0], "OnDemandCost")
	if got := onDemandField.Config.Unit; got != "currencyUSD" {
		t.Fatalf("unit = %q, want currencyUSD", got)
	}
}

func TestConvertReservationCoverageEmptyResponse(t *testing.T) {
	result, err := ConvertReservationCoverage(reservationCoverageQuery(models.FormatTable), &awscostexplorer.GetReservationCoverageOutput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Rows() != 0 {
		t.Fatalf("expected an empty table frame, got %d frames with %d rows", len(result), result[0].Rows())
	}
}

func TestConvertSavingsPlansCoverageEmptyResponse(t *testing.T) {
	result, err := ConvertSavingsPlansCoverage(savingsPlansCoverageQuery(models.FormatTable), &awscostexplorer.GetSavingsPlansCoverageOutput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || result[0].Rows() != 0 {
		t.Fatalf("expected an empty table frame, got %d frames with %d rows", len(result), result[0].Rows())
	}
}
