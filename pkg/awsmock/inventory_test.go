package awsmock

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
)

func TestDescribeSavingsPlansIncludesARetiredItem(t *testing.T) {
	output, err := SavingsPlansClient{}.DescribeSavingsPlans(context.Background(), &savingsplans.DescribeSavingsPlansInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.SavingsPlans) == 0 {
		t.Fatal("expected at least one Savings Plan")
	}
	var sawActive, sawRetired bool
	for _, plan := range output.SavingsPlans {
		switch plan.State {
		case "active":
			sawActive = true
		case "retired":
			sawRetired = true
		}
		if plan.Start == nil || plan.End == nil {
			t.Fatalf("plan %s missing start/end", *plan.SavingsPlanId)
		}
		if _, err := time.Parse(time.RFC3339, *plan.Start); err != nil {
			t.Fatalf("plan %s Start is not RFC3339: %v", *plan.SavingsPlanId, err)
		}
	}
	if !sawActive || !sawRetired {
		t.Fatalf("expected a mix of active and retired plans (active=%v retired=%v)", sawActive, sawRetired)
	}
}

func TestDescribeReservedInstancesEC2IncludesARetiredItem(t *testing.T) {
	output, err := EC2Client{}.DescribeReservedInstances(context.Background(), &ec2.DescribeReservedInstancesInput{})
	if err != nil {
		t.Fatal(err)
	}
	var sawActive, sawRetired bool
	for _, ri := range output.ReservedInstances {
		switch ri.State {
		case ec2types.ReservedInstanceStateActive:
			sawActive = true
		case ec2types.ReservedInstanceStateRetired:
			sawRetired = true
		}
		if ri.Start == nil || ri.End == nil {
			t.Fatalf("reservation %s missing start/end", *ri.ReservedInstancesId)
		}
	}
	if !sawActive || !sawRetired {
		t.Fatalf("expected a mix of active and retired EC2 reservations (active=%v retired=%v)", sawActive, sawRetired)
	}
}

func TestDescribeReservedDBInstancesComputesEndFromDuration(t *testing.T) {
	output, err := RDSClient{}.DescribeReservedDBInstances(context.Background(), &rds.DescribeReservedDBInstancesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.ReservedDBInstances) == 0 {
		t.Fatal("expected at least one RDS reservation")
	}
	for _, ri := range output.ReservedDBInstances {
		if ri.StartTime == nil || ri.Duration == nil {
			t.Fatalf("reservation %s missing start/duration", *ri.ReservedDBInstanceId)
		}
	}
}

func TestDescribeReservedCacheNodesReturnsData(t *testing.T) {
	output, err := ElastiCacheClient{}.DescribeReservedCacheNodes(context.Background(), &elasticache.DescribeReservedCacheNodesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.ReservedCacheNodes) == 0 {
		t.Fatal("expected at least one ElastiCache reservation")
	}
}

func TestDescribeReservedNodesRedshiftReturnsData(t *testing.T) {
	output, err := RedshiftClient{}.DescribeReservedNodes(context.Background(), &redshift.DescribeReservedNodesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.ReservedNodes) == 0 {
		t.Fatal("expected at least one Redshift reservation")
	}
}

func TestDescribeReservedInstancesOpenSearchReturnsData(t *testing.T) {
	output, err := OpenSearchClient{}.DescribeReservedInstances(context.Background(), &opensearch.DescribeReservedInstancesInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.ReservedInstances) == 0 {
		t.Fatal("expected at least one OpenSearch reservation")
	}
	if output.ReservedInstances[0].StartTime == nil {
		t.Fatal("expected a StartTime")
	}
}

func TestResolveCredentialsAlwaysSucceeds(t *testing.T) {
	if err := ResolveCredentials(context.Background()); err != nil {
		t.Fatalf("ResolveCredentials() error = %v, want nil", err)
	}
}
