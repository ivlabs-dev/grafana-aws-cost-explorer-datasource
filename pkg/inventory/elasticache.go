package inventory

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"
)

const elastiCacheMaxPages = 100

// fetchElastiCache lists reserved cache nodes. Like RDS, ElastiCache's
// Describe call has no direct expiry field -- only a start time and a
// duration in seconds -- so End is computed via endFromDuration.
func (s *Service) fetchElastiCache(ctx context.Context) ([]ReservedItem, error) {
	if s.elastiCache == nil {
		return nil, nil
	}

	var items []ReservedItem
	var marker *string
	for page := 1; page <= elastiCacheMaxPages; page++ {
		output, err := s.elastiCache.DescribeReservedCacheNodes(ctx, &elasticache.DescribeReservedCacheNodesInput{Marker: marker})
		if err != nil {
			return nil, err
		}
		for _, reservation := range output.ReservedCacheNodes {
			items = append(items, elastiCacheReservedItem(reservation))
		}
		if output.Marker == nil || *output.Marker == "" {
			return items, nil
		}
		marker = output.Marker
	}
	return nil, fmt.Errorf("%s reservation inventory exceeded the %d-page safety limit", ServiceElastiCache, elastiCacheMaxPages)
}

func elastiCacheReservedItem(reservation types.ReservedCacheNode) ReservedItem {
	var count int32
	if reservation.CacheNodeCount != nil {
		count = *reservation.CacheNodeCount
	}
	var duration int64
	if reservation.Duration != nil {
		duration = int64(*reservation.Duration)
	}
	start := timeValue(reservation.StartTime)
	return ReservedItem{
		Service:      ServiceElastiCache,
		ID:           stringValue(reservation.ReservedCacheNodeId),
		InstanceType: stringValue(reservation.CacheNodeType),
		Count:        count,
		OfferingType: stringValue(reservation.OfferingType),
		State:        stringValue(reservation.State),
		Start:        start,
		End:          endFromDuration(start, duration),
	}
}
