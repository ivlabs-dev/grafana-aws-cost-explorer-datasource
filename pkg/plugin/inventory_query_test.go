package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	spTypes "github.com/aws/aws-sdk-go-v2/service/savingsplans/types"
	"github.com/aws/smithy-go"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/awsclient"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/models"
)

type fakeSavingsPlansDescribe struct{ err error }

func (f fakeSavingsPlansDescribe) DescribeSavingsPlans(
	context.Context,
	*savingsplans.DescribeSavingsPlansInput,
	...func(*savingsplans.Options),
) (*savingsplans.DescribeSavingsPlansOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &savingsplans.DescribeSavingsPlansOutput{
		SavingsPlans: []spTypes.SavingsPlan{{
			SavingsPlanId: aws.String("sp-1"),
			Region:        aws.String("us-east-1"),
			Commitment:    aws.String("1.00"),
			State:         spTypes.SavingsPlanStateActive,
			Start:         aws.String("2026-01-01T00:00:00Z"),
			End:           aws.String("2027-01-01T00:00:00Z"),
		}},
	}, nil
}

type fakeEC2Describe struct{ err error }

func (f fakeEC2Describe) DescribeReservedInstances(
	context.Context,
	*ec2.DescribeReservedInstancesInput,
	...func(*ec2.Options),
) (*ec2.DescribeReservedInstancesOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &ec2.DescribeReservedInstancesOutput{
		ReservedInstances: []ec2types.ReservedInstances{{
			ReservedInstancesId: aws.String("ri-1"),
			InstanceCount:       aws.Int32(1),
		}},
	}, nil
}

type fakeRDSDescribe struct{}

func (fakeRDSDescribe) DescribeReservedDBInstances(
	context.Context,
	*rds.DescribeReservedDBInstancesInput,
	...func(*rds.Options),
) (*rds.DescribeReservedDBInstancesOutput, error) {
	return &rds.DescribeReservedDBInstancesOutput{}, nil
}

type fakeElastiCacheDescribe struct{}

func (fakeElastiCacheDescribe) DescribeReservedCacheNodes(
	context.Context,
	*elasticache.DescribeReservedCacheNodesInput,
	...func(*elasticache.Options),
) (*elasticache.DescribeReservedCacheNodesOutput, error) {
	return &elasticache.DescribeReservedCacheNodesOutput{}, nil
}

type fakeRedshiftDescribe struct{}

func (fakeRedshiftDescribe) DescribeReservedNodes(
	context.Context,
	*redshift.DescribeReservedNodesInput,
	...func(*redshift.Options),
) (*redshift.DescribeReservedNodesOutput, error) {
	return &redshift.DescribeReservedNodesOutput{}, nil
}

type fakeOpenSearchDescribe struct{}

func (fakeOpenSearchDescribe) DescribeReservedInstances(
	context.Context,
	*opensearch.DescribeReservedInstancesInput,
	...func(*opensearch.Options),
) (*opensearch.DescribeReservedInstancesOutput, error) {
	return &opensearch.DescribeReservedInstancesOutput{}, nil
}

func inventoryQueryRequest(queryType string) *backend.QueryDataRequest {
	queryJSON, _ := json.Marshal(models.Query{Version: models.QueryVersion, QueryType: queryType})
	return &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{RefID: "A", JSON: queryJSON}},
	}
}

func TestQueryDataReservationInventory(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{
			client:                  fakeClient{},
			ec2Reservations:         fakeEC2Describe{},
			rdsReservations:         fakeRDSDescribe{},
			elastiCacheReservations: fakeElastiCacheDescribe{},
			redshiftReservations:    fakeRedshiftDescribe{},
			openSearchReservations:  fakeOpenSearchDescribe{},
		},
		log.NewNullLogger(),
	)

	response, err := datasource.QueryData(context.Background(), inventoryQueryRequest(models.QueryTypeReservationInventory))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error != nil || len(got.Frames) != 1 || got.Frames[0].RefID != "A" {
		t.Fatalf("unexpected query response: %+v", got)
	}
	if got.Frames[0].Rows() != 1 {
		t.Fatalf("expected the single EC2 reservation to appear, got %d rows", got.Frames[0].Rows())
	}
}

func TestQueryDataSavingsPlansInventory(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: fakeClient{}, savingsPlans: fakeSavingsPlansDescribe{}},
		log.NewNullLogger(),
	)

	response, err := datasource.QueryData(context.Background(), inventoryQueryRequest(models.QueryTypeSavingsPlansInventory))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error != nil || len(got.Frames) != 1 || got.Frames[0].RefID != "A" {
		t.Fatalf("unexpected query response: %+v", got)
	}
	if got.Frames[0].Rows() != 1 {
		t.Fatalf("expected the single Savings Plan to appear, got %d rows", got.Frames[0].Rows())
	}
}

func TestQueryDataReservationInventorySurfacesHardFailure(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{
			client:                  fakeClient{},
			ec2Reservations:         fakeEC2Describe{err: fmt.Errorf("boom")},
			rdsReservations:         fakeRDSDescribe{},
			elastiCacheReservations: fakeElastiCacheDescribe{},
			redshiftReservations:    fakeRedshiftDescribe{},
			openSearchReservations:  fakeOpenSearchDescribe{},
		},
		log.NewNullLogger(),
	)

	response, err := datasource.QueryData(context.Background(), inventoryQueryRequest(models.QueryTypeReservationInventory))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error == nil {
		t.Fatal("expected an error response for a non-AWS-API EC2 failure")
	}
}

func TestQueryDataInventoryQueriesIgnoreDashboardTimeRange(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: fakeClient{}, savingsPlans: fakeSavingsPlansDescribe{}},
		log.NewNullLogger(),
	)

	request := inventoryQueryRequest(models.QueryTypeSavingsPlansInventory)
	// No TimeRange is set at all; a time-range-scoped query type would
	// resolve an empty/invalid range and short-circuit to an empty result.
	response, err := datasource.QueryData(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error != nil || got.Frames[0].Rows() != 1 {
		t.Fatalf("inventory query should succeed and return data without a time range: %+v", got)
	}
}

// activeEC2Describe returns one active reservation ending in 10 days, or err.
// calls counts DescribeReservedInstances invocations.
type activeEC2Describe struct {
	err   error
	calls *atomic.Int32
}

func (f activeEC2Describe) DescribeReservedInstances(
	context.Context,
	*ec2.DescribeReservedInstancesInput,
	...func(*ec2.Options),
) (*ec2.DescribeReservedInstancesOutput, error) {
	if f.calls != nil {
		f.calls.Add(1)
	}
	if f.err != nil {
		return nil, f.err
	}
	end := time.Now().Add(10 * 24 * time.Hour)
	return &ec2.DescribeReservedInstancesOutput{
		ReservedInstances: []ec2types.ReservedInstances{{
			ReservedInstancesId: aws.String("ri-1"),
			InstanceType:        ec2types.InstanceTypeM5Large,
			InstanceCount:       aws.Int32(1),
			State:               ec2types.ReservedInstanceStateActive,
			End:                 &end,
		}},
	}, nil
}

func alertInventoryRequest(queryType string) *backend.QueryDataRequest {
	queryJSON, _ := json.Marshal(models.Query{Version: models.QueryVersion, QueryType: queryType, Format: models.FormatAlert})
	return &backend.QueryDataRequest{
		Queries: []backend.DataQuery{{RefID: "A", JSON: queryJSON}},
	}
}

func reservationAlertDatasource(ec2 awsclient.EC2ReservedInstancesAPI) *Datasource {
	return newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{
			client:                  fakeClient{},
			ec2Reservations:         ec2,
			rdsReservations:         fakeRDSDescribe{},
			elastiCacheReservations: fakeElastiCacheDescribe{},
			redshiftReservations:    fakeRedshiftDescribe{},
			openSearchReservations:  fakeOpenSearchDescribe{},
		},
		log.NewNullLogger(),
	)
}

func TestQueryDataReservationInventoryAlertFormatReturnsNumericLong(t *testing.T) {
	datasource := reservationAlertDatasource(activeEC2Describe{})

	response, err := datasource.QueryData(context.Background(), alertInventoryRequest(models.QueryTypeReservationInventory))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error != nil || len(got.Frames) != 1 {
		t.Fatalf("unexpected query response: %+v", got)
	}
	frame := got.Frames[0]
	if frame.RefID != "A" || frame.Meta == nil || frame.Meta.Type != data.FrameTypeNumericLong {
		t.Fatalf("expected a numeric-long frame with RefID A, got refID=%q meta=%+v", frame.RefID, frame.Meta)
	}
	if frame.Rows() != 1 {
		t.Fatalf("rows = %d, want 1", frame.Rows())
	}
	if frame.Fields[len(frame.Fields)-1].Name != "days_until_expiry" {
		t.Fatalf("last field = %q, want days_until_expiry", frame.Fields[len(frame.Fields)-1].Name)
	}
}

func TestQueryDataReservationInventoryAlertFormatErrorsOnWarningNotice(t *testing.T) {
	datasource := reservationAlertDatasource(activeEC2Describe{
		err: &smithy.GenericAPIError{Code: "ThrottlingException", Message: "slow down"},
	})

	response, err := datasource.QueryData(context.Background(), alertInventoryRequest(models.QueryTypeReservationInventory))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error == nil || len(got.Frames) != 0 {
		t.Fatalf("expected an error and no frames, got %+v", got)
	}
	if !strings.Contains(got.Error.Error(), "incomplete") || !strings.Contains(got.Error.Error(), "EC2") {
		t.Fatalf("error should say the list is incomplete and name EC2: %v", got.Error)
	}
}

func TestQueryDataReservationInventoryAlertFormatErrorsOnInfoNotice(t *testing.T) {
	datasource := reservationAlertDatasource(activeEC2Describe{
		err: &smithy.GenericAPIError{Code: "UnauthorizedOperation", Message: "denied"},
	})

	response, err := datasource.QueryData(context.Background(), alertInventoryRequest(models.QueryTypeReservationInventory))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error == nil || len(got.Frames) != 0 {
		t.Fatalf("a denied service must fail the alert query, got %+v", got)
	}
}

func TestQueryDataReservationInventoryTableFormatStillReturnsNotices(t *testing.T) {
	datasource := reservationAlertDatasource(activeEC2Describe{
		err: &smithy.GenericAPIError{Code: "UnauthorizedOperation", Message: "denied"},
	})

	response, err := datasource.QueryData(context.Background(), inventoryQueryRequest(models.QueryTypeReservationInventory))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error != nil || len(got.Frames) != 1 {
		t.Fatalf("table format must keep returning a frame, got %+v", got)
	}
	if got.Frames[0].Meta == nil || len(got.Frames[0].Meta.Notices) == 0 {
		t.Fatalf("table format must keep attaching the notice, meta = %+v", got.Frames[0].Meta)
	}
}

func TestQueryDataSavingsPlansInventoryAlertFormat(t *testing.T) {
	datasource := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: fakeClient{}, savingsPlans: fakeSavingsPlansDescribe{}},
		log.NewNullLogger(),
	)
	response, err := datasource.QueryData(context.Background(), alertInventoryRequest(models.QueryTypeSavingsPlansInventory))
	if err != nil {
		t.Fatal(err)
	}
	got := response.Responses["A"]
	if got.Error != nil || len(got.Frames) != 1 || got.Frames[0].Rows() != 1 {
		t.Fatalf("unexpected query response: %+v", got)
	}
	if got.Frames[0].Meta == nil || got.Frames[0].Meta.Type != data.FrameTypeNumericLong {
		t.Fatalf("expected numeric-long, got %+v", got.Frames[0].Meta)
	}

	failing := newDatasource(
		context.Background(),
		validInstanceSettings(),
		fakeFactory{client: fakeClient{}, savingsPlans: fakeSavingsPlansDescribe{err: fmt.Errorf("boom")}},
		log.NewNullLogger(),
	)
	response, err = failing.QueryData(context.Background(), alertInventoryRequest(models.QueryTypeSavingsPlansInventory))
	if err != nil {
		t.Fatal(err)
	}
	if response.Responses["A"].Error == nil {
		t.Fatal("a Savings Plans API failure must fail the alert query")
	}
}

func TestQueryDataInventoryAlertAndTableShareOneUpstreamFetch(t *testing.T) {
	var calls atomic.Int32
	datasource := reservationAlertDatasource(activeEC2Describe{calls: &calls})

	for _, request := range []*backend.QueryDataRequest{
		inventoryQueryRequest(models.QueryTypeReservationInventory),
		alertInventoryRequest(models.QueryTypeReservationInventory),
	} {
		response, err := datasource.QueryData(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		if got := response.Responses["A"]; got.Error != nil {
			t.Fatalf("unexpected error: %v", got.Error)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("EC2 DescribeReservedInstances calls = %d, want 1 (shared cache)", got)
	}
}
