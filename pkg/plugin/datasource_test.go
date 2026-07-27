package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscostexplorer "github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer/types"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
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
	metadata := frameQueryMetadata(t, got.Frames[0])
	if metadata.CacheStatus != "miss" || metadata.PageCount != 1 || metadata.CacheTTLSeconds != 900 {
		t.Fatalf("unexpected first query metadata: %+v", metadata)
	}

	response, err = datasource.QueryData(context.Background(), &backend.QueryDataRequest{
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
	metadata = frameQueryMetadata(t, response.Responses["A"].Frames[0])
	if metadata.CacheStatus != "hit" || metadata.CacheAgeSeconds == nil {
		t.Fatalf("unexpected cached query metadata: %+v", metadata)
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

func TestQueryErrorLogsSafeExecutionMetadata(t *testing.T) {
	const secret = "AKIA-DO-NOT-LOG"
	logger := &recordingLogger{sink: &recordingLogSink{}}
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: errorClient{err: errors.New("transport failed for " + secret)}},
		logger,
	)
	queryJSON, _ := json.Marshal(models.Query{
		Version:     models.QueryVersion,
		Metric:      "UnblendedCost",
		Granularity: models.GranularityDaily,
		Format:      models.FormatTable,
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
	if got.Error == nil || got.Status != backend.StatusBadGateway {
		t.Fatalf("unexpected error response: %+v", got)
	}
	if len(got.Frames) != 0 {
		t.Fatalf("AWS error returned metadata frames: %+v", got.Frames)
	}
	if strings.Contains(got.Error.Error(), secret) {
		t.Fatalf("query error exposed secret: %v", got.Error)
	}

	serialized, err := json.Marshal(logger.sink.snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), secret) {
		t.Fatalf("structured logs exposed secret: %s", serialized)
	}
	for _, expected := range []string{
		"costExplorer.queryExecutedAt",
		"costExplorer.queryDurationMs",
		"costExplorer.cacheStatus",
		"costExplorer.pageCount",
		"error_category",
	} {
		if !strings.Contains(string(serialized), expected) {
			t.Fatalf("structured error log missing %q: %s", expected, serialized)
		}
	}
}

func frameQueryMetadata(t *testing.T, frame *data.Frame) QueryExecutionMetadata {
	t.Helper()
	if frame.Meta == nil {
		t.Fatal("frame metadata is nil")
	}
	custom, ok := frame.Meta.Custom.(map[string]any)
	if !ok {
		t.Fatalf("frame custom metadata type = %T", frame.Meta.Custom)
	}
	metadata, ok := custom["costExplorer"].(QueryExecutionMetadata)
	if !ok {
		t.Fatalf("Cost Explorer metadata type = %T", custom["costExplorer"])
	}
	return metadata
}

type errorClient struct {
	err error
}

func (c errorClient) GetCostAndUsage(
	context.Context,
	*awscostexplorer.GetCostAndUsageInput,
	...func(*awscostexplorer.Options),
) (*awscostexplorer.GetCostAndUsageOutput, error) {
	return nil, c.err
}

type recordingLogSink struct {
	mu      sync.Mutex
	records [][]any
}

func (s *recordingLogSink) add(record []any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, record)
}

func (s *recordingLogSink) snapshot() [][]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([][]any, len(s.records))
	for index, record := range s.records {
		result[index] = append([]any(nil), record...)
	}
	return result
}

type recordingLogger struct {
	sink   *recordingLogSink
	fields []any
}

func (l *recordingLogger) record(level, message string, args ...any) {
	record := append([]any{"level", level, "message", message}, l.fields...)
	record = append(record, args...)
	l.sink.add(record)
}

func (l *recordingLogger) Debug(message string, args ...any) {
	l.record("debug", message, args...)
}

func (l *recordingLogger) Info(message string, args ...any) {
	l.record("info", message, args...)
}

func (l *recordingLogger) Warn(message string, args ...any) {
	l.record("warn", message, args...)
}

func (l *recordingLogger) Error(message string, args ...any) {
	l.record("error", message, args...)
}

func (l *recordingLogger) With(args ...any) log.Logger {
	return &recordingLogger{
		sink:   l.sink,
		fields: append(append([]any(nil), l.fields...), args...),
	}
}

func (l *recordingLogger) Level() log.Level {
	return log.Debug
}

func (l *recordingLogger) FromContext(context.Context) log.Logger {
	return l
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
