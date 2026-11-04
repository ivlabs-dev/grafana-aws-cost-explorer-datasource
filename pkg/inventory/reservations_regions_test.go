package inventory

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/awsclient"
)

type fakeRegionsClient struct {
	calls   atomic.Int32
	err     error
	regions []string
}

func (f *fakeRegionsClient) DescribeRegions(
	context.Context,
	*ec2.DescribeRegionsInput,
	...func(*ec2.Options),
) (*ec2.DescribeRegionsOutput, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	output := &ec2.DescribeRegionsOutput{}
	for _, name := range f.regions {
		output.Regions = append(output.Regions, ec2types.Region{RegionName: aws.String(name)})
	}
	return output, nil
}

// regionalService builds a Service whose home region (us-east-1) has one EC2
// reservation and whose other regions are described by perRegion.
func regionalService(
	t *testing.T,
	regions *fakeRegionsClient,
	perRegion map[string]awsclient.ReservationClients,
) *Service {
	t.Helper()
	service, err := NewService(
		nil,
		&fakeEC2Client{items: []ec2types.ReservedInstances{sampleEC2Reservation("ri-home")}},
		&fakeRDSClient{},
		&fakeElastiCacheClient{},
		&fakeRedshiftClient{},
		&fakeOpenSearchClient{},
		newResultCache(),
		WithRegionalReservations(regions, func(region string) awsclient.ReservationClients {
			if clients, ok := perRegion[region]; ok {
				return clients
			}
			return awsclient.ReservationClients{
				EC2:         &fakeEC2Client{},
				RDS:         &fakeRDSClient{},
				ElastiCache: &fakeElastiCacheClient{},
				Redshift:    &fakeRedshiftClient{},
				OpenSearch:  &fakeOpenSearchClient{},
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestReservationsAllRegionsMergesRegionsAndTagsEachItem(t *testing.T) {
	regions := &fakeRegionsClient{regions: []string{"eu-west-1", "us-east-1", "ap-southeast-2"}}
	service := regionalService(t, regions, map[string]awsclient.ReservationClients{
		"eu-west-1": {
			EC2:         &fakeEC2Client{items: []ec2types.ReservedInstances{sampleEC2Reservation("ri-eu")}},
			RDS:         &fakeRDSClient{},
			ElastiCache: &fakeElastiCacheClient{},
			Redshift:    &fakeRedshiftClient{},
			OpenSearch:  &fakeOpenSearchClient{},
		},
	})

	execution, err := service.Reservations(context.Background(), validSettings(), true)
	if err != nil {
		t.Fatalf("Reservations() error = %v", err)
	}
	if len(execution.Notices) != 0 {
		t.Fatalf("Notices = %+v, want none", execution.Notices)
	}
	got := map[string]string{}
	for _, item := range execution.Items {
		got[item.ID] = item.Region
	}
	if len(got) != 2 || got["ri-home"] != "us-east-1" || got["ri-eu"] != "eu-west-1" {
		t.Fatalf("items by region = %v, want ri-home=us-east-1 and ri-eu=eu-west-1", got)
	}
}

func TestReservationsConfiguredRegionOnlyDoesNotDiscoverRegions(t *testing.T) {
	regions := &fakeRegionsClient{regions: []string{"eu-west-1"}}
	service := regionalService(t, regions, map[string]awsclient.ReservationClients{
		"eu-west-1": {EC2: &fakeEC2Client{items: []ec2types.ReservedInstances{sampleEC2Reservation("ri-eu")}}},
	})

	execution, err := service.Reservations(context.Background(), validSettings(), false)
	if err != nil {
		t.Fatalf("Reservations() error = %v", err)
	}
	if regions.calls.Load() != 0 {
		t.Fatalf("DescribeRegions was called %d times, want 0", regions.calls.Load())
	}
	if len(execution.Items) != 1 || execution.Items[0].Region != "us-east-1" {
		t.Fatalf("Items = %+v, want only the configured region's reservation", execution.Items)
	}
}

func TestReservationsAllRegionsFallsBackWhenDescribeRegionsDenied(t *testing.T) {
	regions := &fakeRegionsClient{err: apiError{code: "UnauthorizedOperation"}}
	service := regionalService(t, regions, nil)

	execution, err := service.Reservations(context.Background(), validSettings(), true)
	if err != nil {
		t.Fatalf("Reservations() error = %v", err)
	}
	if len(execution.Items) != 1 || execution.Items[0].Region != "us-east-1" {
		t.Fatalf("Items = %+v, want the configured region's reservation", execution.Items)
	}
	if len(execution.Notices) != 1 ||
		execution.Notices[0].Severity != data.NoticeSeverityInfo ||
		!strings.Contains(execution.Notices[0].Text, "ec2:DescribeRegions") {
		t.Fatalf("Notices = %+v, want one info notice naming ec2:DescribeRegions", execution.Notices)
	}
}

func TestReservationsAllRegionsOneFailingRegionDoesNotFailTheQuery(t *testing.T) {
	regions := &fakeRegionsClient{regions: []string{"us-east-1", "eu-west-1", "me-south-1"}}
	service := regionalService(t, regions, map[string]awsclient.ReservationClients{
		"me-south-1": {
			EC2:         &fakeEC2Client{err: apiError{code: "AuthFailure"}},
			RDS:         &fakeRDSClient{},
			ElastiCache: &fakeElastiCacheClient{},
			Redshift:    &fakeRedshiftClient{},
			OpenSearch:  &fakeOpenSearchClient{},
		},
	})

	execution, err := service.Reservations(context.Background(), validSettings(), true)
	if err != nil {
		t.Fatalf("Reservations() error = %v", err)
	}
	if len(execution.Items) != 1 {
		t.Fatalf("Items length = %d, want 1", len(execution.Items))
	}
	if len(execution.Notices) != 1 ||
		execution.Notices[0].Severity != data.NoticeSeverityWarning ||
		!strings.Contains(execution.Notices[0].Text, "me-south-1") ||
		!strings.Contains(execution.Notices[0].Text, ServiceEC2) {
		t.Fatalf("Notices = %+v, want one warning naming EC2 and me-south-1", execution.Notices)
	}
}

func TestReservationsAllRegionsAccessDeniedInSomeRegionsNamesThem(t *testing.T) {
	regions := &fakeRegionsClient{regions: []string{"us-east-1", "eu-west-1"}}
	service := regionalService(t, regions, map[string]awsclient.ReservationClients{
		"eu-west-1": {
			EC2:         &fakeEC2Client{err: apiError{code: "UnauthorizedOperation"}},
			RDS:         &fakeRDSClient{},
			ElastiCache: &fakeElastiCacheClient{},
			Redshift:    &fakeRedshiftClient{},
			OpenSearch:  &fakeOpenSearchClient{},
		},
	})

	execution, err := service.Reservations(context.Background(), validSettings(), true)
	if err != nil {
		t.Fatalf("Reservations() error = %v", err)
	}
	if len(execution.Notices) != 1 ||
		execution.Notices[0].Severity != data.NoticeSeverityInfo ||
		!strings.Contains(execution.Notices[0].Text, "eu-west-1") {
		t.Fatalf("Notices = %+v, want one info notice naming eu-west-1", execution.Notices)
	}
}

func TestReservationsAllRegionsAndConfiguredRegionAreCachedSeparately(t *testing.T) {
	regions := &fakeRegionsClient{regions: []string{"us-east-1", "eu-west-1"}}
	service := regionalService(t, regions, map[string]awsclient.ReservationClients{
		"eu-west-1": {EC2: &fakeEC2Client{items: []ec2types.ReservedInstances{sampleEC2Reservation("ri-eu")}}},
	})

	single, err := service.Reservations(context.Background(), validSettings(), false)
	if err != nil {
		t.Fatal(err)
	}
	all, err := service.Reservations(context.Background(), validSettings(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(single.Items) != 1 || len(all.Items) != 2 {
		t.Fatalf("single = %d items, all = %d items; want 1 and 2 (a shared cache entry would repeat the first result)", len(single.Items), len(all.Items))
	}
}
