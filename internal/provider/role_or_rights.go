// Copyright (c) 2026 Pete Beegle
// SPDX-License-Identifier: MPL-2.0

package provider

// useRoleIDOverAdminRights decides, for a resource with an
// ExactlyOneOf(role_id, admin_rights) config validator, whether applying an
// API response to state should populate role_id (true) or admin_rights
// (false).
//
// If the current model already has role_id set, or has neither set (a fresh
// import) and the API response has a role_id, role_id wins and admin_rights
// is cleared; otherwise admin_rights wins and role_id is cleared. This
// preserves whichever of the two the user configured instead of re-deriving
// it from the API response on every read.
func useRoleIDOverAdminRights(stateRoleIDNull, stateAdminRightsNull, apiHasRoleID bool) bool {
	return !stateRoleIDNull || (stateRoleIDNull && stateAdminRightsNull && apiHasRoleID)
}
