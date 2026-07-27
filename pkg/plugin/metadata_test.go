package plugin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/costexplorer"
)

func TestQueryExecutionMetadataCacheMiss(t *testing.T) {
	executedAt := time.Date(2026, 7, 27, 15, 4, 5, 6, time.FixedZone("CEST", 2*60*60))
	metadata := NewQueryExecutionMetadata(
		&costexplorer.Execution{
			CacheResult:           cache.ResultMiss,
			CacheTTL:              15 * time.Minute,
			Pages:                 3,
			AWSDuration:           1250 * time.Millisecond,
			ContainsEstimatedData: true,
		},
		executedAt,
		1500*time.Millisecond,
		true,
	)

	if metadata.QueryExecutedAt != "2026-07-27T13:04:05.000000006Z" {
		t.Fatalf("queryExecutedAt = %q", metadata.QueryExecutedAt)
	}
	if metadata.CacheStatus != "miss" || metadata.CacheAgeSeconds != nil {
		t.Fatalf("unexpected miss metadata: %+v", metadata)
	}
	if metadata.CacheTTLSeconds != 900 || metadata.PageCount != 3 {
		t.Fatalf("unexpected cache/page metadata: %+v", metadata)
	}
	if metadata.AWSAPIDurationMS != 1250 || metadata.QueryDurationMS != 1500 {
		t.Fatalf("unexpected duration metadata: %+v", metadata)
	}
	if !metadata.IncludesIncompletePeriod || !metadata.ContainsEstimatedData {
		t.Fatalf("unexpected result flags: %+v", metadata)
	}
}

func TestQueryExecutionMetadataCacheHitAgeAndSharedMapping(t *testing.T) {
	for _, result := range []cache.Result{cache.ResultHit, cache.ResultShared} {
		t.Run(string(result), func(t *testing.T) {
			metadata := NewQueryExecutionMetadata(
				&costexplorer.Execution{
					CacheResult: result,
					CacheAge:    42*time.Second + 900*time.Millisecond,
					CacheTTL:    15 * time.Minute,
				},
				time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC),
				100*time.Millisecond,
				false,
			)

			if metadata.CacheStatus != "hit" {
				t.Fatalf("cacheStatus = %q, want hit", metadata.CacheStatus)
			}
			if metadata.CacheAgeSeconds == nil || *metadata.CacheAgeSeconds != 42 {
				t.Fatalf("cacheAgeSeconds = %v, want 42", metadata.CacheAgeSeconds)
			}
		})
	}
}

func TestAttachQueryExecutionMetadataPreservesFrameMetadata(t *testing.T) {
	frame := data.NewFrame("cost")
	frame.Meta = &data.FrameMeta{
		Custom: map[string]any{"existing": "value"},
		Notices: []data.Notice{{
			Severity: data.NoticeSeverityInfo,
			Text:     "existing notice",
		}},
	}
	metadata := NewQueryExecutionMetadata(
		&costexplorer.Execution{CacheResult: cache.ResultMiss, CacheTTL: time.Minute},
		time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC),
		time.Second,
		false,
	)

	AttachQueryExecutionMetadata(data.Frames{frame}, metadata)

	custom, ok := frame.Meta.Custom.(map[string]any)
	if !ok {
		t.Fatalf("custom metadata type = %T", frame.Meta.Custom)
	}
	if custom["existing"] != "value" {
		t.Fatalf("existing custom metadata was not preserved: %+v", custom)
	}
	if _, ok := custom["costExplorer"].(QueryExecutionMetadata); !ok {
		t.Fatalf("Cost Explorer metadata missing: %+v", custom)
	}
	if len(frame.Meta.Notices) != 1 || frame.Meta.Notices[0].Text != "existing notice" {
		t.Fatalf("frame notices changed: %+v", frame.Meta.Notices)
	}
}

func TestQueryMetadataAndLogFieldsExcludeSensitiveValues(t *testing.T) {
	metadata := NewQueryExecutionMetadata(
		&costexplorer.Execution{
			CacheResult: cache.ResultHit,
			CacheAge:    time.Second,
			CacheTTL:    time.Minute,
			Pages:       1,
		},
		time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC),
		time.Second,
		false,
	)
	frame := data.NewFrame("cost")
	AttachQueryExecutionMetadata(data.Frames{frame}, metadata)

	serialized, err := json.Marshal(struct {
		Frame  *data.Frame `json:"frame"`
		Fields []any       `json:"logFields"`
	}{
		Frame:  frame,
		Fields: metadata.LogFields(),
	})
	if err != nil {
		t.Fatal(err)
	}

	lower := strings.ToLower(string(serialized))
	for _, forbidden := range []string{
		"accesskey",
		"secret",
		"sessiontoken",
		"externalid",
		"credentialcontext",
		"cachekey",
		"authorization",
		"tagvalue",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("serialized metadata/log fields contain forbidden term %q: %s", forbidden, serialized)
		}
	}
}

func TestQueryExecutionMetadataWithoutCacheExecutionIsBypass(t *testing.T) {
	metadata := NewQueryExecutionMetadata(
		nil,
		time.Date(2026, 7, 27, 13, 0, 0, 0, time.UTC),
		-time.Second,
		false,
	)
	if metadata.CacheStatus != "bypass" || metadata.QueryDurationMS != 0 {
		t.Fatalf("unexpected nil execution metadata: %+v", metadata)
	}
}
