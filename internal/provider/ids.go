// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"strings"
)

// joinID joins parts into a single Terraform resource ID, separated by ":".
func joinID(parts ...string) string {
	return strings.Join(parts, ":")
}

// splitID splits a Terraform import ID into exactly n non-empty parts,
// separated by ":". format describes the expected shape (e.g.
// "project_id:language_id") for use in the error message.
func splitID(id string, n int, format string) ([]string, error) {
	parts := strings.Split(id, ":")
	if len(parts) != n {
		return nil, fmt.Errorf("unexpected import ID %q, expected %s", id, format)
	}
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("unexpected import ID %q, expected %s", id, format)
		}
	}
	return parts, nil
}
