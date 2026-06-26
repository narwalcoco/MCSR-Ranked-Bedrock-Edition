package store

import (
	"fmt"
	"strconv"
	"time"
)

// scanTime converts a value returned by modernc.org/sqlite into time.Time.
//
// Why this helper exists: even with INTEGER + unixepoch() columns,
// modernc.org/sqlite has been observed to return datetime-like values as
// driver.Value strings instead of int64 — the choice depends on the
// column's declared type and current lib version. To handle both, we
// inside the `string` arm first try Unix-epoch numeric parsing, then
// fall back to SQLite's `datetime('now')` text format.
//
// Inputs (everything else is an error):
//   - nil          → zero time, no error
//   - int64 / int  → Unix epoch seconds, UTC
//   - string       → numeric first (digits-only → Unix seconds), then
//                    `2006-01-02 15:04:05`
func scanTime(v any) (time.Time, error) {
	if v == nil {
		return time.Time{}, nil
	}
	switch x := v.(type) {
	case int64:
		return time.Unix(x, 0).UTC(), nil
	case int:
		return time.Unix(int64(x), 0).UTC(), nil
	case string:
		if t, err := parseUnixSeconds(x); err == nil {
			return t, nil
		}
		return parseSQLDateTime(x)
	default:
		return time.Time{}, fmt.Errorf("scanTime: unsupported scan type %T", v)
	}
}

// parseUnixSeconds accepts a string of decimal digits (optionally signed)
// representing Unix epoch seconds and returns the corresponding time.UTC().
func parseUnixSeconds(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty")
	}
	// Reject anything not pure digits or signed digits — avoids misreading
	// a real formatted datetime like "2024-01-02 12:34:56" as 2024 epoch.
	for _, r := range s {
		if r < '0' || r > '9' {
			return time.Time{}, fmt.Errorf("not numeric")
		}
	}
	secs, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(secs, 0).UTC(), nil
}

// parseSQLDateTime parses a string in SQLite's default datetime format.
func parseSQLDateTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("scanTime: bad time %q: %w", s, err)
	}
	return t.UTC(), nil
}

// scanTimePtr is a convenience for nullable columns: writes nil when the
// column is NULL, otherwise the parsed time. Used for started_at / ended_at.
func scanTimePtr(v any, out **time.Time) error {
	if v == nil {
		*out = nil
		return nil
	}
	t, err := scanTime(v)
	if err != nil {
		return err
	}
	*out = &t
	return nil
}
