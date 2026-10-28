package inventory

import "time"

// timeValue dereferences an AWS SDK *time.Time field, treating a nil
// pointer as the zero time rather than an error.
func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

// endFromDuration computes an expiry for the four services whose Describe
// APIs return only a start time and a duration rather than a direct end
// field (RDS, ElastiCache, Redshift, OpenSearch).
func endFromDuration(start time.Time, durationSeconds int64) time.Time {
	if start.IsZero() {
		return time.Time{}
	}
	return start.Add(time.Duration(durationSeconds) * time.Second)
}
