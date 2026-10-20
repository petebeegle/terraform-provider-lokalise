// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package client_test

import (
	"context"
	"testing"

	"github.com/petebeegle/lokalise-provider/internal/client"
	"github.com/petebeegle/lokalise-provider/internal/lokalisefake"
)

func TestTeamUsers_List(t *testing.T) {
	ctx := context.Background()
	fake := lokalisefake.New(t)
	c := client.New(fake.URL(), "tok", client.WithRateLimit(1000, 1000))

	userID := fake.AddTeamUser(lokalisefake.DefaultTeamID, "b@example.com")

	users, err := c.ListTeamUsers(ctx, lokalisefake.DefaultTeamID)
	if err != nil {
		t.Fatalf("ListTeamUsers() error = %v", err)
	}

	var found *client.TeamUser
	for i := range users {
		if users[i].UserID == userID {
			found = &users[i]
		}
	}
	if found == nil {
		t.Fatalf("ListTeamUsers() = %+v, want user %d", users, userID)
	}
	if found.Email != "b@example.com" {
		t.Fatalf("found.Email = %q, want %q", found.Email, "b@example.com")
	}
	if found.Role == "" {
		t.Fatalf("found.Role is empty")
	}
}
