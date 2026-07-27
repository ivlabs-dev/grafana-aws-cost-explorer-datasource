package plugin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

type dashboardLifecycleClient struct {
	calls int
}

func (c *dashboardLifecycleClient) GetCostAndUsage(
	_ context.Context,
	_ *awscostexplorer.GetCostAndUsageInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetCostAndUsageOutput, error) {
	c.calls++
	return &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{
			{
				TimePeriod: &types.DateInterval{Start: aws.String("2026-07-01")},
				Estimated:  true,
				Groups: []types.Group{
					dashboardServiceGroup("Amazon EC2", "10"),
					dashboardServiceGroup("Amazon S3", "1"),
				},
			},
			{
				TimePeriod: &types.DateInterval{Start: aws.String("2026-07-02")},
				Groups: []types.Group{
					dashboardServiceGroup("Amazon EC2", "2"),
					dashboardServiceGroup("Amazon S3", "9"),
				},
			},
		},
	}, nil
}

func TestDashboardQueryLifecycleReusesAWSResponseAcrossPresentations(t *testing.T) {
	client := &dashboardLifecycleClient{}
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: client},
		log.NewNullLogger(),
	)
	datasource.now = func() time.Time {
		return time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	}
	timeRange := backend.TimeRange{
		From: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
	}

	tableQuery, err := json.Marshal(models.Query{
		Version:      models.QueryVersion,
		Metric:       "UnblendedCost",
		Granularity:  models.GranularityDaily,
		GroupBy:      []string{"SERVICE"},
		Format:       models.FormatTable,
		TopN:         1,
		IncludeOther: aws.Bool(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	tableResponse, err := datasource.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{RefID: "A", JSON: tableQuery, TimeRange: timeRange}},
	})
	if err != nil {
		t.Fatal(err)
	}
	tableResult := tableResponse.Responses["A"]
	if tableResult.Error != nil || len(tableResult.Frames) != 1 {
		t.Fatalf("unexpected table response: %+v", tableResult)
	}
	tableFrame := tableResult.Frames[0]
	if tableFrame.Rows() != 4 || tableFrame.Fields[0].Name != "Period" {
		t.Fatalf("unexpected table shape: fields=%d rows=%d", len(tableFrame.Fields), tableFrame.Rows())
	}
	wantRows := map[string]float64{
		"2026-07-01|Amazon EC2": 10,
		"2026-07-01|Other":      1,
		"2026-07-02|Amazon EC2": 2,
		"2026-07-02|Other":      9,
	}
	for row := 0; row < tableFrame.Rows(); row++ {
		key := tableFrame.Fields[0].At(row).(string) + "|" + tableFrame.Fields[1].At(row).(string)
		if got, ok := wantRows[key]; !ok || tableFrame.Fields[3].At(row).(float64) != got {
			t.Fatalf("unexpected table row %q: %+v", key, tableFrame.Fields[3].At(row))
		}
		delete(wantRows, key)
	}
	if len(wantRows) != 0 {
		t.Fatalf("missing table rows: %+v", wantRows)
	}
	tableMetadata := frameQueryMetadata(t, tableFrame)
	if tableMetadata.CacheStatus != "miss" || tableMetadata.PageCount != 1 ||
		!tableMetadata.ContainsEstimatedData {
		t.Fatalf("unexpected table metadata: %+v", tableMetadata)
	}

	seriesQuery, err := json.Marshal(models.Query{
		Version:     models.QueryVersion,
		Metric:      "UnblendedCost",
		Granularity: models.GranularityDaily,
		GroupBy:     []string{"SERVICE"},
		Format:      models.FormatTimeSeries,
	})
	if err != nil {
		t.Fatal(err)
	}
	seriesResponse, err := datasource.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{RefID: "B", JSON: seriesQuery, TimeRange: timeRange}},
	})
	if err != nil {
		t.Fatal(err)
	}
	seriesResult := seriesResponse.Responses["B"]
	if seriesResult.Error != nil || len(seriesResult.Frames) != 2 {
		t.Fatalf("unexpected time-series response: %+v", seriesResult)
	}
	names := map[string]bool{}
	for _, frame := range seriesResult.Frames {
		names[frame.Name] = true
		if got := frame.Fields[1].Labels["SERVICE"]; got != frame.Name {
			t.Fatalf("series %q lost its SERVICE label: %q", frame.Name, got)
		}
	}
	if !names["Amazon EC2"] || !names["Amazon S3"] {
		t.Fatalf("unexpected clean series names: %+v", names)
	}
	seriesMetadata := frameQueryMetadata(t, seriesResult.Frames[0])
	if seriesMetadata.CacheStatus != "hit" || seriesMetadata.CacheAgeSeconds == nil ||
		seriesMetadata.PageCount != 0 || !seriesMetadata.ContainsEstimatedData {
		t.Fatalf("unexpected cached series metadata: %+v", seriesMetadata)
	}
	if client.calls != 1 {
		t.Fatalf("AWS calls = %d, want one shared cached request", client.calls)
	}
}

func dashboardServiceGroup(service, amount string) types.Group {
	return types.Group{
		Keys: []string{service},
		Metrics: map[string]types.MetricValue{
			"UnblendedCost": {
				Amount: aws.String(amount),
				Unit:   aws.String("USD"),
			},
		},
	}
}
