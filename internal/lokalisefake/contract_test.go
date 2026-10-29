// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package lokalisefake_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/petebeegle/terraform-provider-lokalise/internal/lokalisefake"
)

// TestFake_MatchesOpenAPIExampleShapes asserts that the fake's responses
// have the same envelope and object keys as the OpenAPI response examples
// in internal/client/testdata/openapi (which the client's contract tests
// decode), so provider tests run against the documented shapes.
func TestFake_MatchesOpenAPIExampleShapes(t *testing.T) {
	fake := lokalisefake.New(t)
	team := lokalisefake.DefaultTeamID
	teamStr := strconv.FormatInt(team, 10)

	call := func(method, path string, body any) map[string]any {
		t.Helper()
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, fake.URL()+path, rd)
		req.Header.Set("X-Api-Token", "tok")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, raw)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("%s %s: decode: %v", method, path, err)
		}
		return out
	}

	proj := call("POST", "/projects", map[string]any{"name": "p", "base_lang_iso": "en", "languages": []any{map[string]any{"lang_iso": "en"}}})
	projectID := proj["project_id"].(string)

	langCreate := call("POST", "/projects/"+projectID+"/languages", map[string]any{"languages": []any{map[string]any{"lang_iso": "fr"}}})
	langID := strconv.FormatInt(int64(langCreate["languages"].([]any)[0].(map[string]any)["lang_id"].(float64)), 10)
	langGet := call("GET", "/projects/"+projectID+"/languages/"+langID, nil)
	langUpdate := call("PUT", "/projects/"+projectID+"/languages/"+langID, map[string]any{"lang_iso": "fr", "lang_name": "French", "plural_forms": []string{"one", "other"}})

	contribCreate := call("POST", "/projects/"+projectID+"/contributors", map[string]any{"contributors": []any{map[string]any{
		"email": "a@example.com", "fullname": "A", "role_id": 1,
		"languages": []any{map[string]any{"lang_iso": "en", "is_writable": true}},
	}}})
	userID := strconv.FormatInt(int64(contribCreate["contributor"].(map[string]any)["user_id"].(float64)), 10)
	contribList := call("GET", "/projects/"+projectID+"/contributors", nil)
	contribGet := call("GET", "/projects/"+projectID+"/contributors/"+userID, nil)
	contribUpdate := call("PUT", "/projects/"+projectID+"/contributors/"+userID, map[string]any{"role_id": 1, "languages": []any{map[string]any{"lang_iso": "en", "is_writable": true}}})

	groupBody := map[string]any{"name": "g", "role_id": 1, "languages": map[string]any{"reference": []int64{}, "contributable": []int64{640}}}
	groupCreate := call("POST", "/teams/"+teamStr+"/groups", groupBody)
	groupID := strconv.FormatInt(int64(groupCreate["group"].(map[string]any)["group_id"].(float64)), 10)
	groupGet := call("GET", "/teams/"+teamStr+"/groups/"+groupID, nil)
	groupUpdate := call("PUT", "/teams/"+teamStr+"/groups/"+groupID, groupBody)
	membersAdd := call("PUT", "/teams/"+teamStr+"/groups/"+groupID+"/members/add", map[string]any{"users": []int64{22212}})
	projectsAdd := call("PUT", "/teams/"+teamStr+"/groups/"+groupID+"/projects/add", map[string]any{"projects": []string{projectID}})
	roles := call("GET", "/teams/"+teamStr+"/roles", nil)
	users := call("GET", "/teams/"+teamStr+"/users", nil)

	// Keys present in the examples that the fake does not model and the
	// client does not read. ("permissions" is a string array on a role; a
	// group's permissions object is checked via the paths below.)
	ignore := map[string]bool{
		"created_at": true, "created_at_timestamp": true, "permissions": true,
		"is_admin": true, "is_reviewer": true, "is_reviwer": true,
		"lang_iso": true, "lang_name": true,
		"description": true, "tag": true, "tagColor": true, "doesEnableAllReadOnlyLanguages": true,
	}

	cases := []struct {
		example string
		got     map[string]any
		// paths are dot-separated paths whose object keys are compared;
		// "" is the top level. A numeric segment indexes an array.
		paths []string
	}{
		{"language_create.json", langCreate, []string{"", "languages.0"}},
		{"language_get.json", langGet, []string{"", "language"}},
		{"language_update.json", langUpdate, []string{"", "language"}},
		{"contributor_create.json", contribCreate, []string{"", "contributor", "contributor.languages.0"}},
		{"contributor_list.json", contribList, []string{"", "contributors.0", "contributors.0.languages.0"}},
		{"contributor_get.json", contribGet, []string{"", "contributor", "contributor.languages.0"}},
		{"contributor_update.json", contribUpdate, []string{"", "contributor", "contributor.languages.0"}},
		{"group_create.json", groupCreate, []string{"", "group", "group.permissions", "group.permissions.languages.0"}},
		{"group_get.json", groupGet, []string{"", "permissions", "permissions.languages.0"}},
		{"group_update.json", groupUpdate, []string{"", "group"}},
		{"group_members_add.json", membersAdd, []string{"", "group"}},
		{"group_projects_add.json", projectsAdd, []string{"", "group"}},
		{"role_list.json", roles, []string{"", "roles.0"}},
		{"team_user_list.json", users, []string{"", "team_users.0"}},
	}

	for _, tc := range cases {
		t.Run(tc.example, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "client", "testdata", "openapi", tc.example))
			if err != nil {
				t.Fatal(err)
			}
			var want map[string]any
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			for _, p := range tc.paths {
				wantObj, err := at(want, p)
				if err != nil {
					t.Fatalf("example %s: %v", p, err)
				}
				gotObj, err := at(tc.got, p)
				if err != nil {
					t.Fatalf("fake response %s: %v (response: %v)", p, err, tc.got)
				}
				for _, k := range sortedKeys(wantObj) {
					if ignore[k] {
						continue
					}
					if _, ok := gotObj[k]; !ok {
						t.Errorf("path %q: fake response lacks key %q present in the OpenAPI example (got keys %v)", p, k, sortedKeys(gotObj))
					}
				}
			}
		})
	}

	// Specific encodings the examples use, which the fake must reproduce so
	// the client's tolerance for them is exercised by provider tests.
	if v := contribList["contributors"].([]any)[0].(map[string]any)["languages"].([]any)[0].(map[string]any)["is_writable"]; v != float64(1) {
		t.Errorf("contributor list is_writable = %#v, want 1", v)
	}
	if v, ok := groupGet["admin_rights"]; ok {
		t.Errorf("group GET has top-level admin_rights %v; the example nests it under permissions", v)
	}
	if v := groupCreate["group"].(map[string]any)["permissions"].(map[string]any)["admin_rights"]; fmt.Sprint(v) != "map[]" {
		t.Errorf("empty group admin_rights = %#v, want {}", v)
	}
}

func at(v map[string]any, path string) (map[string]any, error) {
	if path == "" {
		return v, nil
	}
	var cur any = v
	for _, seg := range strings.Split(path, ".") {
		switch c := cur.(type) {
		case map[string]any:
			cur = c[seg]
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i >= len(c) {
				return nil, fmt.Errorf("bad index %q", seg)
			}
			cur = c[i]
		default:
			return nil, fmt.Errorf("cannot descend into %T at %q", cur, seg)
		}
	}
	m, ok := cur.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%q is %T, not an object", path, cur)
	}
	return m, nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
