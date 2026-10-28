// Package inventory lists what Reservations and Savings Plans an account
// actually owns -- distinct from pkg/costexplorer, which reports how well
// they're being utilized. It talks to six AWS services that have nothing to
// do with Cost Explorer (the Savings Plans service, plus the per-service
// Reserved-Instance "Describe" calls for EC2, RDS, ElastiCache, Redshift,
// and OpenSearch), and normalizes their differently-shaped responses into
// two flat lists with a computed expiry so a future alert rule can
// threshold on "days until expiry".
package inventory

import "time"

// ReservedItem is one reservation from any of the five reservation-eligible
// services, normalized to a common shape. Four of the five underlying AWS
// APIs don't return an expiry date directly -- only a start time and a
// duration -- so End is always computed (Start + Duration) rather than
// trusted as a literal AWS field.
type ReservedItem struct {
	Service      string
	ID           string
	InstanceType string
	Count        int32
	OfferingType string
	State        string
	Start        time.Time
	End          time.Time
}

// SavingsPlan is one Savings Plan owned by the account, normalized from the
// AWS Savings Plans service's DescribeSavingsPlans response.
type SavingsPlan struct {
	ID            string
	Type          string
	PaymentOption string
	Region        string
	Commitment    float64
	State         string
	Start         time.Time
	End           time.Time
}

const (
	ServiceEC2         = "EC2"
	ServiceRDS         = "RDS"
	ServiceElastiCache = "ElastiCache"
	ServiceRedshift    = "Redshift"
	ServiceOpenSearch  = "OpenSearch"
)
