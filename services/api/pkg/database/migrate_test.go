package database

import "testing"

func TestToPgxURLRewritesOnlyThePostgresSchemes(t *testing.T) {
	cases := map[string]string{
		"postgres://u:p@localhost:5432/logistics_dev?sslmode=disable": "pgx5://u:p@localhost:5432/logistics_dev?sslmode=disable",
		"postgresql://u:p@localhost:5432/logistics_dev":               "pgx5://u:p@localhost:5432/logistics_dev",
		"pgx5://u:p@localhost:5432/logistics_dev":                     "pgx5://u:p@localhost:5432/logistics_dev",
	}
	for input, want := range cases {
		if got := toPgxURL(input); got != want {
			t.Errorf("toPgxURL(%q) = %q, want %q", input, got, want)
		}
	}
}
