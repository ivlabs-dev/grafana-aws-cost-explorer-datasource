package plugin

import (
	"encoding/json"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/costexplorer"
)

// QueryExecutionMetadata is intentionally limited to non-sensitive execution
// facts. Query filters, credential context, cache keys, and AWS request details
// must never be added to this structure.
type QueryExecutionMetadata struct {
	QueryExecutedAt          string `json:"queryExecutedAt"`
	AWSAPIDurationMS         int64  `json:"awsApiDurationMs"`
	QueryDurationMS          int64  `json:"queryDurationMs"`
	CacheStatus              string `json:"cacheStatus"`
	CacheAgeSeconds          *int64 `json:"cacheAgeSeconds,omitempty"`
	CacheTTLSeconds          int64  `json:"cacheTtlSeconds"`
	PageCount                int    `json:"pageCount"`
	IncludesIncompletePeriod bool   `json:"includesIncompletePeriod"`
	ContainsEstimatedData    bool   `json:"containsEstimatedData"`
}

// NewQueryExecutionMetadata builds the metadata attached to every successful
// query frame. Callers supply the exact incomplete-period decision so period
// resolution can evolve independently from metadata generation.
func NewQueryExecutionMetadata(
	execution *costexplorer.Execution,
	executedAt time.Time,
	queryDuration time.Duration,
	includesIncompletePeriod bool,
) QueryExecutionMetadata {
	metadata := QueryExecutionMetadata{
		QueryExecutedAt:          executedAt.UTC().Format(time.RFC3339Nano),
		QueryDurationMS:          nonNegativeDuration(queryDuration).Milliseconds(),
		CacheStatus:              string(cache.ResultBypass),
		IncludesIncompletePeriod: includesIncompletePeriod,
	}
	if execution == nil {
		return metadata
	}

	status := execution.CacheStatus()
	metadata.AWSAPIDurationMS = nonNegativeDuration(execution.AWSDuration).Milliseconds()
	metadata.CacheStatus = string(status)
	metadata.CacheTTLSeconds = int64(nonNegativeDuration(execution.CacheTTL) / time.Second)
	metadata.PageCount = execution.Pages
	metadata.ContainsEstimatedData = execution.ContainsEstimatedData
	if status == cache.ResultHit {
		age := int64(nonNegativeDuration(execution.CacheAge) / time.Second)
		metadata.CacheAgeSeconds = &age
	}
	return metadata
}

// AttachQueryExecutionMetadata merges the Cost Explorer namespace into each
// frame's SDK-supported custom metadata without replacing notices or unrelated
// custom metadata.
func AttachQueryExecutionMetadata(frames data.Frames, metadata QueryExecutionMetadata) {
	for _, frame := range frames {
		if frame.Meta == nil {
			frame.Meta = &data.FrameMeta{}
		}
		frame.Meta.Custom = mergeCustomMetadata(frame.Meta.Custom, metadata)
	}
}

func (m QueryExecutionMetadata) LogFields() []any {
	fields := []any{
		"costExplorer.queryExecutedAt", m.QueryExecutedAt,
		"costExplorer.awsApiDurationMs", m.AWSAPIDurationMS,
		"costExplorer.queryDurationMs", m.QueryDurationMS,
		"costExplorer.cacheStatus", m.CacheStatus,
		"costExplorer.cacheTtlSeconds", m.CacheTTLSeconds,
		"costExplorer.pageCount", m.PageCount,
		"costExplorer.includesIncompletePeriod", m.IncludesIncompletePeriod,
		"costExplorer.containsEstimatedData", m.ContainsEstimatedData,
	}
	if m.CacheAgeSeconds != nil {
		fields = append(fields, "costExplorer.cacheAgeSeconds", *m.CacheAgeSeconds)
	}
	return fields
}

func mergeCustomMetadata(existing any, metadata QueryExecutionMetadata) map[string]any {
	custom := make(map[string]any)
	if existing != nil {
		encoded, err := json.Marshal(existing)
		if err == nil {
			_ = json.Unmarshal(encoded, &custom)
		}
	}
	custom["costExplorer"] = metadata
	return custom
}

func nonNegativeDuration(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	return value
}
