// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"strconv"
)

// TeamUser is a user known to a Lokalise team, as returned by
// GET /teams/{team_id}/users.
type TeamUser struct {
	UserID   int64
	Email    string
	Fullname string
	Role     string
}

// teamUserWire is the wire shape of one entry of GET /teams/{team_id}/users'
// "team_users" array (OpenAPI "TeamUsers" schema, subset).
type teamUserWire struct {
	UserID   int64  `json:"user_id"`
	Email    string `json:"email"`
	Fullname string `json:"fullname"`
	Role     string `json:"role"`
}

func (w teamUserWire) toTeamUser() TeamUser {
	return TeamUser(w)
}

// ListTeamUsers lists every user known to teamID.
func (c *HTTPClient) ListTeamUsers(ctx context.Context, teamID int64) ([]TeamUser, error) {
	wires, err := listAll[teamUserWire](ctx, c, "/teams/"+strconv.FormatInt(teamID, 10)+"/users", "team_users")
	if err != nil {
		return nil, err
	}
	users := make([]TeamUser, 0, len(wires))
	for _, w := range wires {
		users = append(users, w.toTeamUser())
	}
	return users, nil
}
