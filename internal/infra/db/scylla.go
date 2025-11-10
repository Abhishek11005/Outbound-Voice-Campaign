package db

import (
	"fmt"

	"github.com/gocql/gocql"

	"github.com/acme/outbound-call-campaign/internal/config"
)

// Scylla wraps a gocql session.
type Scylla struct {
	session *gocql.Session
}

// NewScylla creates a new Scylla session.
func NewScylla(cfg config.ScyllaConfig) (*Scylla, error) {
	cluster := gocql.NewCluster(cfg.Hosts...)
	cluster.Port = cfg.Port
	cluster.Keyspace = cfg.Keyspace
	cluster.Consistency = parseConsistency(cfg.Consistency)
	cluster.Timeout = cfg.Timeout
	cluster.RetryPolicy = &gocql.SimpleRetryPolicy{NumRetries: 3}

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("scylla: create session: %w", err)
	}

	return &Scylla{session: session}, nil
}

// Session exposes the gocql session.
func (s *Scylla) Session() *gocql.Session {
	return s.session
}

// Close shuts down the session.
func (s *Scylla) Close() error {
	if s.session != nil {
		s.session.Close()
	}
	return nil
}

func parseConsistency(level string) gocql.Consistency {
	switch level {
	case "one":
		return gocql.One
	case "local_quorum":
		return gocql.LocalQuorum
	case "local_one":
		return gocql.LocalOne
	case "each_quorum":
		return gocql.EachQuorum
	case "quorum":
		fallthrough
	default:
		return gocql.Quorum
	}
}
