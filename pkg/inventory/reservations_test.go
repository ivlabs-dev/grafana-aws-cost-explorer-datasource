package inventory

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
)

func sampleEC2Reservation(id string) ec2types.ReservedInstances {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	return ec2types.ReservedInstances{
		ReservedInstancesId: aws.String(id),
		InstanceType:        ec2types.InstanceTypeM5Large,
		InstanceCount:       aws.Int32(2),
		OfferingType:        ec2types.OfferingTypeValuesAllUpfront,
		State:               ec2types.ReservedInstanceStateActive,
		Start:               &start,
		End:                 &end,
	}
}

func sampleRDSReservation(id string) rdstypes.ReservedDBInstance {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return rdstypes.ReservedDBInstance{
		ReservedDBInstanceId: aws.String(id),
		DBInstanceClass:      aws.String("db.r5.large"),
		DBInstanceCount:      aws.Int32(1),
		OfferingType:         aws.String("All Upfront"),
		State:                aws.String("active"),
		StartTime:            &start,
		Duration:             aws.Int32(31536000), // 1 year, in seconds
	}
}

func TestReservationsMergesAllServices(t *testing.T) {
	service, err := NewService(
		nil,
		&fakeEC2Client{items: []ec2types.ReservedInstances{sampleEC2Reservation("ri-1")}},
		&fakeRDSClient{items: []rdstypes.ReservedDBInstance{sampleRDSReservation("ri-rds-1")}},
		&fakeElastiCacheClient{},
		&fakeRedshiftClient{},
		&fakeOpenSearchClient{},
		newResultCache(),
	)
	if err != nil {
		t.Fatal(err)
	}

	execution, err := service.Reservations(context.Background(), validSettings())
	if err != nil {
		t.Fatalf("Reservations() error = %v", err)
	}
	if len(execution.Items) != 2 {
		t.Fatalf("Items length = %d, want 2; got %+v", len(execution.Items), execution.Items)
	}
	if len(execution.Notices) != 0 {
		t.Fatalf("Notices = %+v, want none", execution.Notices)
	}

	var ec2Item, rdsItem *ReservedItem
	for i := range execution.Items {
		switch execution.Items[i].Service {
		case ServiceEC2:
			ec2Item = &execution.Items[i]
		case ServiceRDS:
			rdsItem = &execution.Items[i]
		}
	}
	if ec2Item == nil || ec2Item.ID != "ri-1" || ec2Item.Count != 2 {
		t.Fatalf("unexpected EC2 item = %+v", ec2Item)
	}
	if ec2Item.End.Year() != 2027 {
		t.Fatalf("EC2 End should use AWS's direct field, got %v", ec2Item.End)
	}
	if rdsItem == nil || rdsItem.ID != "ri-rds-1" {
		t.Fatalf("unexpected RDS item = %+v", rdsItem)
	}
	wantRDSEnd := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	if !rdsItem.End.Equal(wantRDSEnd) {
		t.Fatalf("RDS End should be computed from Start + Duration, got %v, want %v", rdsItem.End, wantRDSEnd)
	}
}

func TestReservationsExcludesAccessDeniedServicesWithAnInfoNotice(t *testing.T) {
	service, err := NewService(
		nil,
		&fakeEC2Client{items: []ec2types.ReservedInstances{sampleEC2Reservation("ri-1")}},
		&fakeRDSClient{err: apiError{code: "AccessDeniedException"}},
		&fakeElastiCacheClient{err: apiError{code: "AccessDeniedException"}},
		&fakeRedshiftClient{err: apiError{code: "AccessDeniedException"}},
		&fakeOpenSearchClient{err: apiError{code: "AccessDeniedException"}},
		newResultCache(),
	)
	if err != nil {
		t.Fatal(err)
	}

	execution, err := service.Reservations(context.Background(), validSettings())
	if err != nil {
		t.Fatalf("Reservations() error = %v", err)
	}
	if len(execution.Items) != 1 {
		t.Fatalf("Items length = %d, want 1 (only EC2 succeeded)", len(execution.Items))
	}
	// Excluded from the list, but not silently -- an Info notice per denied
	// service distinguishes "zero reservations" from "zero permission",
	// which is otherwise impossible to tell apart from the panel.
	if len(execution.Notices) != 4 {
		t.Fatalf("Notices length = %d, want 4 (one per access-denied service), got %+v", len(execution.Notices), execution.Notices)
	}
	for _, notice := range execution.Notices {
		if notice.Severity != data.NoticeSeverityInfo {
			t.Fatalf("access-denied notice severity = %v, want info; got %+v", notice.Severity, notice)
		}
	}
}

func TestReservationsAllAccessDeniedIsEmptyNotHardFailure(t *testing.T) {
	denied := apiError{code: "AccessDeniedException"}
	service, err := NewService(
		nil,
		&fakeEC2Client{err: denied},
		&fakeRDSClient{err: denied},
		&fakeElastiCacheClient{err: denied},
		&fakeRedshiftClient{err: denied},
		&fakeOpenSearchClient{err: denied},
		newResultCache(),
	)
	if err != nil {
		t.Fatal(err)
	}

	execution, err := service.Reservations(context.Background(), validSettings())
	if err != nil {
		t.Fatalf("Reservations() error = %v, want a successful empty result when every service is merely unauthorized", err)
	}
	if len(execution.Items) != 0 || len(execution.Notices) != 5 {
		t.Fatalf("expected 0 items and 5 info notices, got %+v", execution)
	}
}

func TestReservationsAttachesNoticeForNonPermissionError(t *testing.T) {
	service, err := NewService(
		nil,
		&fakeEC2Client{items: []ec2types.ReservedInstances{sampleEC2Reservation("ri-1")}},
		&fakeRDSClient{err: apiError{code: "ThrottlingException"}},
		&fakeElastiCacheClient{},
		&fakeRedshiftClient{},
		&fakeOpenSearchClient{},
		newResultCache(),
	)
	if err != nil {
		t.Fatal(err)
	}

	execution, err := service.Reservations(context.Background(), validSettings())
	if err != nil {
		t.Fatalf("Reservations() error = %v", err)
	}
	if len(execution.Items) != 1 {
		t.Fatalf("Items length = %d, want 1", len(execution.Items))
	}
	if len(execution.Notices) != 1 {
		t.Fatalf("Notices length = %d, want 1", len(execution.Notices))
	}
	if execution.Notices[0].Severity != data.NoticeSeverityWarning {
		t.Fatalf("Notice severity = %v, want warning", execution.Notices[0].Severity)
	}
}

func TestReservationsFailsWhenAllServicesError(t *testing.T) {
	throttled := apiError{code: "ThrottlingException"}
	service, err := NewService(
		nil,
		&fakeEC2Client{err: throttled},
		&fakeRDSClient{err: throttled},
		&fakeElastiCacheClient{err: throttled},
		&fakeRedshiftClient{err: throttled},
		&fakeOpenSearchClient{err: throttled},
		newResultCache(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Reservations(context.Background(), validSettings()); err == nil {
		t.Fatal("expected an error when every reservation source fails")
	}
}

func TestReservationsFailsFastOnNonAWSError(t *testing.T) {
	service, err := NewService(
		nil,
		&fakeEC2Client{err: errBoom},
		&fakeRDSClient{},
		&fakeElastiCacheClient{},
		&fakeRedshiftClient{},
		&fakeOpenSearchClient{},
		newResultCache(),
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.Reservations(context.Background(), validSettings()); err == nil {
		t.Fatal("expected a hard failure for a non-AWS-API error")
	}
}

func TestReservationsNilClientsAreTreatedAsNotConfigured(t *testing.T) {
	service, err := NewService(nil, nil, nil, nil, nil, nil, newResultCache())
	if err != nil {
		t.Fatal(err)
	}

	execution, err := service.Reservations(context.Background(), validSettings())
	if err != nil {
		t.Fatalf("Reservations() error = %v", err)
	}
	if len(execution.Items) != 0 || len(execution.Notices) != 0 {
		t.Fatalf("expected an empty, notice-free result when no clients are configured, got %+v", execution)
	}
}

func TestReservationsCaches(t *testing.T) {
	ec2Client := &fakeEC2Client{items: []ec2types.ReservedInstances{sampleEC2Reservation("ri-1")}}
	service, err := NewService(nil, ec2Client, &fakeRDSClient{}, &fakeElastiCacheClient{}, &fakeRedshiftClient{}, &fakeOpenSearchClient{}, newResultCache())
	if err != nil {
		t.Fatal(err)
	}

	first, err := service.Reservations(context.Background(), validSettings())
	if err != nil {
		t.Fatal(err)
	}
	if first.CacheResult != cache.ResultMiss {
		t.Fatalf("first call cache result = %v, want miss", first.CacheResult)
	}

	second, err := service.Reservations(context.Background(), validSettings())
	if err != nil {
		t.Fatal(err)
	}
	if second.CacheResult != cache.ResultHit {
		t.Fatalf("second call cache result = %v, want hit", second.CacheResult)
	}
}
