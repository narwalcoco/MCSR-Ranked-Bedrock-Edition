package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// decodeJSON parses the request body into v. The body reader is closed
// automatically when decoding finishes.
func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return errors.New("missing request body")
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty request body")
		}
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

// writeError writes a structured JSON error response with status code.
// `detail` is included only when non-nil so safe data still hits clients.
func writeError(w http.ResponseWriter, status int, msg string, detail error) {
	body := map[string]any{"error": msg}
	if detail != nil {
		body["detail"] = detail.Error()
	}
	writeJSON(w, status, body)
}

// parseInt64 parses an int64 from a path-captured string, returning a
// descriptive error suitable for the API when input is bad.
func parseInt64(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("empty id")
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q: %w", s, err)
	}
	return v, nil
}
