package inventory

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans"
	"github.com/aws/aws-sdk-go-v2/service/savingsplans/types"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
)

func TestSavingsPlansHappyPathAndCaching(t *testing.T) {
	client := &fakeSavingsPlansClient{
		fn: func(*savingsplans.DescribeSavingsPlansInput) (*savingsplans.DescribeSavingsPlansOutput, error) {
			return &savingsplans.DescribeSavingsPlansOutput{SavingsPlans: []types.SavingsPlan{sampleSavingsPlan("sp-1")}}, nil
		},
	}
	service, err := NewService(client, nil, nil, nil, nil, nil, newResultCache())
	if err != nil {
		t.Fatal(err)
	}

	execution, err := service.SavingsPlans(context.Background(), validSettings())
	if err != nil {
		t.Fatalf("SavingsPlans() error = %v", err)
	}
	if execution.CacheResult != cache.ResultMiss {
		t.Fatalf("first call cache result = %v, want miss", execution.CacheResult)
	}
	if len(execution.Plans) != 1 {
		t.Fatalf("Plans length = %d, want 1", len(execution.Plans))
	}
	plan := execution.Plans[0]
	if plan.ID != "sp-1" || plan.Region != "us-east-1" || plan.Commitment != 12.50 || plan.State != "active" {
		t.Fatalf("unexpected plan = %+v", plan)
	}
	if plan.Start.IsZero() || plan.End.IsZero() {
		t.Fatalf("expected parsed start/end, got %+v", plan)
	}

	execution, err = service.SavingsPlans(context.Background(), validSettings())
	if err != nil {
		t.Fatal(err)
	}
	if execution.CacheResult != cache.ResultHit {
		t.Fatalf("second call cache result = %v, want hit", execution.CacheResult)
	}
	if client.calls != 1 {
		t.Fatalf("AWS calls = %d, want 1 (second call should be served from cache)", client.calls)
	}
}

func TestSavingsPlansPaginates(t *testing.T) {
	calls := 0
	client := &fakeSavingsPlansClient{
		fn: func(input *savingsplans.DescribeSavingsPlansInput) (*savingsplans.DescribeSavingsPlansOutput, error) {
			calls++
			if input.NextToken == nil {
				return &savingsplans.DescribeSavingsPlansOutput{
					SavingsPlans: []types.SavingsPlan{sampleSavingsPlan("sp-1")},
					NextToken:    aws.String("page-2"),
				}, nil
			}
			return &savingsplans.DescribeSavingsPlansOutput{SavingsPlans: []types.SavingsPlan{sampleSavingsPlan("sp-2")}}, nil
		},
	}
	service, err := NewService(client, nil, nil, nil, nil, nil, newResultCache())
	if err != nil {
		t.Fatal(err)
	}

	execution, err := service.SavingsPlans(context.Background(), validSettings())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("AWS calls = %d, want 2", calls)
	}
	if len(execution.Plans) != 2 {
		t.Fatalf("Plans length = %d, want 2", len(execution.Plans))
	}
}

func TestSavingsPlansClassifiesAccessDenied(t *testing.T) {
	client := &fakeSavingsPlansClient{
		fn: func(*savingsplans.DescribeSavingsPlansInput) (*savingsplans.DescribeSavingsPlansOutput, error) {
			return nil, apiError{code: "AccessDeniedException"}
		},
	}
	service, err := NewService(client, nil, nil, nil, nil, nil, newResultCache())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.SavingsPlans(context.Background(), validSettings()); err == nil {
		t.Fatal("expected a classified error for an access-denied Savings Plans request")
	}
}

func TestSavingsPlansWithoutClientReturnsEmpty(t *testing.T) {
	service, err := NewService(nil, nil, nil, nil, nil, nil, newResultCache())
	if err != nil {
		t.Fatal(err)
	}

	execution, err := service.SavingsPlans(context.Background(), validSettings())
	if err != nil {
		t.Fatalf("SavingsPlans() error = %v", err)
	}
	if len(execution.Plans) != 0 {
		t.Fatalf("Plans = %+v, want empty", execution.Plans)
	}
}
