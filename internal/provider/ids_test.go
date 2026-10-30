// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package provider

import "testing"

func TestSplitID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		n       int
		format  string
		want    []string
		wantErr bool
	}{
		{name: "two parts", id: "a:b", n: 2, format: "project_id:language_id", want: []string{"a", "b"}},
		{name: "three parts", id: "a:b:c", n: 3, format: "team_id:group_id:user_id", want: []string{"a", "b", "c"}},
		{name: "too few parts", id: "a", n: 2, format: "project_id:language_id", wantErr: true},
		{name: "empty part", id: "a::c", n: 3, format: "team_id:group_id:user_id", wantErr: true},
		{name: "too many parts", id: "a:b:c", n: 2, format: "project_id:language_id", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitID(tt.id, tt.n, tt.format)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("splitID(%q, %d, %q) = %v, nil; want error", tt.id, tt.n, tt.format, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitID(%q, %d, %q) unexpected error: %v", tt.id, tt.n, tt.format, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("splitID(%q, %d, %q) = %v, want %v", tt.id, tt.n, tt.format, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitID(%q, %d, %q) = %v, want %v", tt.id, tt.n, tt.format, got, tt.want)
				}
			}
		})
	}
}
