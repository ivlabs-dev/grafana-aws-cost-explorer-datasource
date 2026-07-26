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
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/awsclient"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

type fakeFactory struct {
	client     awsclient.CostExplorerAPI
	resolveErr error
}

func (f fakeFactory) New(context.Context, models.PluginSettings) (*awsclient.Bundle, error) {
	return &awsclient.Bundle{
		CostExplorer: f.client,
		ResolveCredentials: func(context.Context) error {
			return f.resolveErr
		},
	}, nil
}

type fakeClient struct{}

func (fakeClient) GetCostAndUsage(
	_ context.Context,
	input *awscostexplorer.GetCostAndUsageInput,
	_ ...func(*awscostexplorer.Options),
) (*awscostexplorer.GetCostAndUsageOutput, error) {
	return &awscostexplorer.GetCostAndUsageOutput{
		ResultsByTime: []types.ResultByTime{{
			TimePeriod: input.TimePeriod,
			Total: map[string]types.MetricValue{
				input.Metrics[0]: {Amount: aws.String("10.25"), Unit: aws.String("USD")},
			},
		}},
	}, nil
}

func TestQueryDataReturnsFrame(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: fakeClient{}},
		log.NewNullLogger(),
	)
	queryJSON, _ := json.Marshal(models.Query{
		Version:     models.QueryVersion,
		Metric:      "UnblendedCost",
		Granularity: models.GranularityDaily,
		Format:      models.FormatTimeSeries,
	})
	response, err := datasource.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{
			RefID: "A",
			JSON:  queryJSON,
			TimeRange: backend.TimeRange{
				From: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
				To:   time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC),
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error != nil || len(got.Frames) != 1 || got.Frames[0].RefID != "A" {
		t.Fatalf("unexpected query response: %+v", got)
	}
}

func TestCheckHealthResolvesCredentialsAndQueriesCostExplorer(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: fakeClient{}},
		log.NewNullLogger(),
	)
	result, err := datasource.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != backend.HealthStatusOk {
		t.Fatalf("health status = %s, message = %s", result.Status, result.Message)
	}
}

func validInstanceSettings() backend.DataSourceInstanceSettings {
	return backend.DataSourceInstanceSettings{
		UID: "test",
		JSONData: []byte(`{
			"authMode": "default",
			"region": "us-east-1",
			"cacheTTLSeconds": 900,
			"cacheMaxEntries": 256
		}`),
		DecryptedSecureJSONData: map[string]string{},
	}
}
