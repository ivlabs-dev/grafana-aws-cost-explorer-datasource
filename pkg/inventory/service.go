package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/smithy-go"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/awsclient"
	"github.com/ivlabs-dev/grafana-aws-cost-explorer-datasource/pkg/cache"
)

// Service lists owned Savings Plans and Reservations. Unlike
// pkg/costexplorer.Service, a nil client for any one of these six AWS
// integrations is not a construction error -- it means that integration
// wasn't wired up (impossible in production, where every client is built
// from the same already-resolved AWS config, but convenient for tests that
// only care about a subset of this package's surface). At call time, a nil
// client for one of the five reservation-eligible services is treated the
// same as that service's IAM permission being withheld: it's silently
// excluded from the merged list rather than failing the whole query.
type Service struct {
	savingsPlans awsclient.SavingsPlansAPI
	ec2          awsclient.EC2ReservedInstancesAPI
	rds          awsclient.RDSReservedInstancesAPI
	elastiCache  awsclient.ElastiCacheReservedNodesAPI
	redshift     awsclient.RedshiftReservedNodesAPI
	openSearch   awsclient.OpenSearchReservedInstancesAPI
	cache        cache.Cache
}

func NewService(
	savingsPlans awsclient.SavingsPlansAPI,
	ec2Client awsclient.EC2ReservedInstancesAPI,
	rdsClient awsclient.RDSReservedInstancesAPI,
	elastiCacheClient awsclient.ElastiCacheReservedNodesAPI,
	redshiftClient awsclient.RedshiftReservedNodesAPI,
	openSearchClient awsclient.OpenSearchReservedInstancesAPI,
	resultCache cache.Cache,
) (*Service, error) {
	if resultCache == nil {
		return nil, fmt.Errorf("cache must not be nil")
	}
	return &Service{
		savingsPlans: savingsPlans,
		ec2:          ec2Client,
		rds:          rdsClient,
		elastiCache:  elastiCacheClient,
		redshift:     redshiftClient,
		openSearch:   openSearchClient,
		cache:        resultCache,
	}, nil
}

// Execution carries the same cache/timing facts as
// pkg/costexplorer.Execution so NewQueryExecutionMetadata can report cache
// status and timing for inventory queries without changing its signature.
type Execution struct {
	CacheResult cache.Result
	CacheAge    time.Duration
	CacheTTL    time.Duration
	AWSDuration time.Duration
}

func cacheKey(credentialContext, discriminator string) (string, error) {
	payload := struct {
		CredentialContext string `json:"credentialContext"`
		Discriminator     string `json:"discriminator"`
	}{
		CredentialContext: credentialContext,
		Discriminator:     discriminator,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode inventory cache key: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// isAccessDenied reports whether err is an AWS access-denied class error --
// the expected, non-exceptional outcome when an account hasn't granted one
// of this feature's six optional IAM permissions.
func isAccessDenied(err error) bool {
	var apiError smithy.APIError
	if !errors.As(err, &apiError) {
		return false
	}
	switch strings.ToLower(apiError.ErrorCode()) {
	case "accessdenied", "accessdeniedexception", "unauthorizedexception", "unauthorizedoperation":
		return true
	default:
		return false
	}
}

// isAWSAPIError reports whether err is a classifiable AWS API error (of any
// kind), as opposed to a network/transport or context error. Only AWS API
// errors are safe to downgrade to a per-service Notice; anything else
// aborts the whole inventory query instead of silently hiding it.
func isAWSAPIError(err error) bool {
	var apiError smithy.APIError
	return errors.As(err, &apiError)
}

// classifyError adapts an AWS or transport error into a message safe to
// return to the caller, naming the specific service for context.
func classifyError(service string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%s reservation inventory request was canceled: %w", service, err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%s reservation inventory request timed out: %w", service, err)
	}

	var apiError smithy.APIError
	if !errors.As(err, &apiError) {
		return fmt.Errorf("%s reservation inventory request failed; verify network connectivity and the configured AWS endpoint", service)
	}
	if isAccessDenied(err) {
		return fmt.Errorf("AWS denied the %s reservation inventory request; grant the relevant Describe permission", service)
	}
	return fmt.Errorf("%s reservation inventory request returned AWS API error %s", service, apiError.ErrorCode())
}
