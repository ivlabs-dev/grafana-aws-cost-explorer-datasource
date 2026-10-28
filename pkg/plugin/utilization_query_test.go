package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

// utilizationClient provides realistic Reservation/Savings Plans utilization
// responses for the QueryData dispatch tests below; everything else falls
// through to fakeClient's defaults.
type utilizationClient struct {
	fakeClient
}

func (utilizationClient) GetReservationUtilization(
	_ context.Context,
	_ *awscostexplorer.GetReservationUtilizationInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetReservationUtilizationOutput, error) {
	return &awscostexplorer.GetReservationUtilizationOutput{
		UtilizationsByTime: []types.UtilizationByTime{{
			TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
			Total:      &types.ReservationAggregates{UtilizationPercentage: aws.String("80")},
		}},
	}, nil
}

func (utilizationClient) GetSavingsPlansUtilization(
	_ context.Context,
	_ *awscostexplorer.GetSavingsPlansUtilizationInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetSavingsPlansUtilizationOutput, error) {
	return &awscostexplorer.GetSavingsPlansUtilizationOutput{
		SavingsPlansUtilizationsByTime: []types.SavingsPlansUtilizationByTime{{
			TimePeriod:  &types.DateInterval{Start: aws.String("2026-07-01")},
			Utilization: &types.SavingsPlansUtilization{UtilizationPercentage: aws.String("95")},
		}},
	}, nil
}

func (utilizationClient) GetReservationCoverage(
	_ context.Context,
	_ *awscostexplorer.GetReservationCoverageInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetReservationCoverageOutput, error) {
	return &awscostexplorer.GetReservationCoverageOutput{
		CoveragesByTime: []types.CoverageByTime{{
			TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
			Total:      &types.Coverage{CoverageHours: &types.CoverageHours{CoverageHoursPercentage: aws.String("40%")}},
		}},
	}, nil
}

func (utilizationClient) GetSavingsPlansCoverage(
	_ context.Context,
	_ *awscostexplorer.GetSavingsPlansCoverageInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetSavingsPlansCoverageOutput, error) {
	return &awscostexplorer.GetSavingsPlansCoverageOutput{
		SavingsPlansCoverages: []types.SavingsPlansCoverage{{
			TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
			Coverage:   &types.SavingsPlansCoverageData{CoveragePercentage: aws.String("52%")},
		}},
	}, nil
}

func utilizationQueryRequest(queryType string) *backend.QueryDataRequest {
	queryJSON, _ := json.Marshal(models.Query{
		Version:     models.QueryVersion,
		QueryType:   queryType,
		Granularity: models.GranularityDaily,
		Format:      models.FormatTimeSeries,
	})
	return &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{
			RefID: "A",
			JSON:  queryJSON,
			TimeRange: backend.TimeRange{
				From: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
			},
		}},
	}
}

func TestQueryDataReservationUtilization(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: utilizationClient{}},
		log.NewNullLogger(),
	)

	response, err := datasource.QueryData(context.Background(), utilizationQueryRequest(models.QueryTypeReservationUtilization))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error != nil || len(got.Frames) != 1 || got.Frames[0].RefID != "A" {
		t.Fatalf("unexpected query response: %+v", got)
	}
}

func TestQueryDataSavingsPlansUtilization(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: utilizationClient{}},
		log.NewNullLogger(),
	)

	response, err := datasource.QueryData(context.Background(), utilizationQueryRequest(models.QueryTypeSavingsPlansUtilization))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error != nil || len(got.Frames) != 1 || got.Frames[0].RefID != "A" {
		t.Fatalf("unexpected query response: %+v", got)
	}
}

func TestQueryDataReservationUtilizationSurfacesAWSError(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: errorClient{err: fmt.Errorf("boom")}},
		log.NewNullLogger(),
	)

	response, err := datasource.QueryData(context.Background(), utilizationQueryRequest(models.QueryTypeReservationUtilization))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error == nil {
		t.Fatal("expected an error response")
	}
}

func TestQueryDataReservationCoverage(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: utilizationClient{}},
		log.NewNullLogger(),
	)

	response, err := datasource.QueryData(context.Background(), utilizationQueryRequest(models.QueryTypeReservationCoverage))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error != nil || len(got.Frames) != 1 || got.Frames[0].RefID != "A" {
		t.Fatalf("unexpected query response: %+v", got)
	}
}

func TestQueryDataSavingsPlansCoverage(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: utilizationClient{}},
		log.NewNullLogger(),
	)

	response, err := datasource.QueryData(context.Background(), utilizationQueryRequest(models.QueryTypeSavingsPlansCoverage))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error != nil || len(got.Frames) != 1 || got.Frames[0].RefID != "A" {
		t.Fatalf("unexpected query response: %+v", got)
	}
}

func TestQueryDataReservationCoverageSurfacesAWSError(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: errorClient{err: fmt.Errorf("boom")}},
		log.NewNullLogger(),
	)

	response, err := datasource.QueryData(context.Background(), utilizationQueryRequest(models.QueryTypeReservationCoverage))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error == nil {
		t.Fatal("expected an error response")
	}
}
