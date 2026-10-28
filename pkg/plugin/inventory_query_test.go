package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	spTypes "github.com/aws/aws-sdk-go-v2/service/savingsplans/types"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
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
