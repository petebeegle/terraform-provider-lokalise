// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// flexSlice is a JSON array that also accepts the PHP-style encodings the
// Lokalise API uses in its OpenAPI examples: an empty array serialized as
// `{}` (e.g. "admin_rights": {}, "members": {}), an object keyed by index,
// and null. Object values are decoded in key order.
type flexSlice[T any] []T

func (s *flexSlice[T]) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		*s = nil
		return nil
	}
	switch b[0] {
	case '[':
		var items []T
		if err := json.Unmarshal(b, &items); err != nil {
			return err
		}
		*s = items
		return nil
	case '{':
		var m map[string]T
		if err := json.Unmarshal(b, &m); err != nil {
			return err
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		items := make([]T, 0, len(m))
		for _, k := range keys {
			items = append(items, m[k])
		}
		*s = items
		return nil
	default:
		return fmt.Errorf("lokalise: expected array or object, got %s", b)
	}
}

// flexBool is a JSON boolean that also accepts 0/1 integers (e.g.
// "is_writable": 1 in list responses) and null (false).
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	switch string(bytes.TrimSpace(b)) {
	case "true", "1", `"1"`, `"true"`:
		*f = true
	case "false", "0", "null", `"0"`, `"false"`, `""`:
		*f = false
	default:
		return fmt.Errorf("lokalise: expected boolean, got %s", b)
	}
	return nil
}
