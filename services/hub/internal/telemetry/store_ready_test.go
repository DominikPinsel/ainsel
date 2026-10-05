package telemetry

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// A store is a metrics backend only if it can actually read. NewStore tolerates
// a nil pool so wiring never has to special-case a hub without a database, but
// the API gate must not count that store as a source: a hub with neither
// Prometheus nor a database has to answer "no metrics backend configured", not
// advertise "postgres" and then fail every query with ErrNoDatabase.
func TestReadyReportsOnlyStoresThatCanQuery(t *testing.T) {
	tests := []struct {
		name  string
		store *Store
		want  bool
	}{
		{"no store at all", nil, false},
		{"store built without a pool", NewStore(nil), false},
		{"store built over a pool", NewStore(&pgxpool.Pool{}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.store.Ready(); got != tt.want {
				t.Errorf("Ready() = %v, want %v", got, tt.want)
			}
		})
	}
}
