package sqlite

import (
	"runtime"
)

// suggestConnectionCount calculates the optimal number
// of parallel connections to the database.
// Source: https://github.com/nalgeon/redka/blob/017c0b28f7685311c3948b2e6a531012c8092bd3/internal/sqlx/db.go#L225
func suggestConnectionCount() int {
	// Benchmarks show that setting nConns>2 does not significantly
	// improve throughput, so I'm not sure what the best value is.
	// For now, I'm setting it to 2-8 depending on the number of CPUs.
	cpuCount := runtime.NumCPU()
	switch {
	case cpuCount < 2:
		return 2
	case cpuCount > 8:
		return 8
	default:
		return cpuCount
	}
}
