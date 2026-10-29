# Manual verification checklist

The fake server (`internal/lokalisefake`) was built from the Lokalise OpenAPI
spec and documentation, not from live traffic. Every wire shape it assumes is
listed below.

Response envelopes are pinned to the OpenAPI spec's response **examples**:
`internal/client/testdata/openapi/` holds verbatim copies, the client's
contract tests (`internal/client/contract_test.go`) decode each one through
the client, and `internal/lokalisefake/contract_test.go` checks the fake
emits the same keys. Items marked "fixed from the OpenAPI examples" were
wrong in an earlier version of the client and fake (which agreed with each
other, so no test caught it); they still need a live check. Run this checklist once against a real Lokalise team with a
**throwaway project** before relying on the provider for real projects, and
after any Lokalise API change.

For each item: run the HCL or curl command shown, compare against what the
provider assumes, and fill in the blank "Observed:" line. If anything
disagrees with the assumption, file a follow-up to fix `internal/client` and
`internal/lokalisefake` before trusting `terraform plan` output.

Set these first:

```shell
export LOKALISE_API_TOKEN=<a real token, team Admin>
export TEAM_ID=<your team id>
```

---

## 1. Project create/read response shape

Assumption: `POST /projects` and `GET /projects/{id}` both return the project
object at the top level (not wrapped in `{"project": ...}`), with
`base_language_iso` present on GET.

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"tf-manual-check","base_lang_iso":"en"}' \
  https://api.lokalise.com/api2/projects | tee /tmp/project.json

curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/projects/$(jq -r .project_id /tmp/project.json)"
```

Observed: 2026-10-29: as assumed. POST and GET both return the project unwrapped, with `base_language_iso`.## 2. `base_language_iso` normalization

Assumption: the base ISO code the server returns on GET equals the
`base_lang_iso` sent on create, verbatim. If Lokalise normalizes case or
separators (e.g. `en_US` → `en-us`), `lokalise_project`'s
`base_language_iso` (a `RequiresReplace` attribute) will show a perpetual
replace diff on every plan, because Terraform compares the normalized GET
value against the literal config value.

```hcl
resource "lokalise_project" "check" {
  name              = "tf-manual-check-norm"
  base_language_iso = "en_US"
}
```

Run `terraform apply` then `terraform plan` again with no config change.

Observed: 2026-10-29: `en_US` round-trips verbatim; no diff on re-plan.## 3. Duplicate project names

Assumption: project names are not unique; the real API's behavior on a
duplicate name (second create with the same `name`) is unknown — does it
succeed, as the fake assumes?

Observed: 2026-10-29: duplicate names are accepted (200, new `project_id`).## 3a. Empty `description` clears the field on update

Assumption: `lokalise_project`'s Create omits `description` from the request
body when empty, but Update always sends `description` (including `""`), so
setting a project's `description` back to empty in config actually clears
it server-side rather than being ignored.

```shell
curl -s -X PUT -H "X-Api-Token: $LOKALISE_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"tf-manual-check","description":""}' \
  "https://api.lokalise.com/api2/projects/<project_id>"
```

Then GET the project and confirm `description` is now empty, not unchanged.

Observed: 2026-10-29: `description: ""` on PUT clears it; GET returns `""`.## 3b. `team_id` response type

Assumption: `team_id` always comes back as a JSON number that fits in
int64. The OpenAPI spec allows the create request to take either a number
or a UUID string for `team_id`; the provider only ever sends a number, but
confirm GET never returns a UUID-string form instead of a number (which
would break `lokalise_project`'s `types.Int64` field).

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/projects/<project_id>" | jq '.team_id | type'
```

Expect `"number"`.

Observed: 2026-10-29: `"number"`.## 3c. `project_type` default when omitted

Assumption: when Create omits `project_type` entirely, the server defaults
it to `localization_files`. `lokalise_project` has no client-side default:
an omitted `project_type` is sent as absent and the server's value is
stored (and kept for imported projects of any type).

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"tf-manual-check-type","base_lang_iso":"en"}' \
  https://api.lokalise.com/api2/projects
```

Confirm the returned `project_type` is `localization_files`.

Observed: 2026-10-29: `localization_files`.## 4. Language list/create/get/update/delete shapes

Assumption: `GET /projects/{id}/languages` returns
`{"project_id":.., "languages":[{"lang_id","lang_iso","lang_name","is_rtl","plural_forms"}]}`.
`POST /projects/{id}/languages` body is
`{"languages":[{"lang_iso","custom_name","custom_plural_forms"}]}`, response
shaped the same as the list (client takes the first element).
`GET` and `PUT /projects/{id}/languages/{lang_id}` return the language
wrapped: `{"project_id":.., "language":{...}}` (fixed from the OpenAPI
examples; previously assumed unwrapped). `PUT` sends only changed fields
(`lang_iso`, `lang_name`, `plural_forms`), never empty overwrites.

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/projects/<project_id>/languages"

curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"languages":[{"lang_iso":"fr"}]}' \
  "https://api.lokalise.com/api2/projects/<project_id>/languages"

curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/projects/<project_id>/languages/<lang_id>" | jq 'keys'
```

Expect `["language","project_id"]` for the single-language GET.

Observed: 2026-10-29: as assumed. Single-language GET keys are `language`, `project_id` (plus `project_uuid`). The list and POST entries also carry `project_language_id` as a string.## 5. `lang_iso` mutability via PUT

Assumption: `PUT /projects/{id}/languages/{language_id}` accepts `lang_iso`
in the body and the provider treats `lang_iso` as updatable in place (no
`RequiresReplace`). Real language-update endpoints typically only accept
name/plural-form changes — if the real API silently ignores `lang_iso` on
PUT, a config change to `lang_iso` will show no error but the ISO code won't
actually change server-side, and the provider would need `RequiresReplace`
added instead.

```shell
curl -s -X PUT -H "X-Api-Token: $LOKALISE_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"lang_iso":"fr-FR"}' \
  "https://api.lokalise.com/api2/projects/<project_id>/languages/<language_id>"
```

Then `GET` the same language and confirm whether `lang_iso` actually changed.

Observed: 2026-10-29: PUT `{"lang_iso":"fr_FR"}` changes it (GET confirms), so in-place update is correct. Lokalise uses `_`, not `-`.## 5a. `custom_name` empty-string handling on language create

Assumption: `CreateLanguageInput.CustomName` is sent via `omitempty`, so a
`custom_name = ""` in `lokalise_project_language` config omits the field
from the request entirely, and Lokalise then defaults the display name to
the ISO code (rather than erroring, or setting a literal empty display
name).

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"languages":[{"lang_iso":"de"}]}' \
  "https://api.lokalise.com/api2/projects/<project_id>/languages"
```

Confirm the returned `lang_name` for the new language defaults to `de`.

Observed: 2026-10-29: `lang_name` defaults to Lokalise's display name (`German`), not the ISO code.## 5b. Project-language `lang_id` space

Assumption: the fake synthesizes `lang_id` as an FNV hash of the ISO code
(e.g. not a real system ID). Confirm what a real project language's
`lang_id` actually is — specifically, whether it equals the same
system-wide language catalogue ID returned by `GET /system/languages` for
that ISO (e.g. `en` = `640`), which matters if `lokalise_project_language`
or the group-language resources ever need to cross-reference the two ID
spaces.

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/projects/<project_id>/languages"

curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/system/languages" | jq '.languages[] | select(.lang_iso=="en")'
```

Compare the `lang_id` values from both calls for the same ISO code.
Also note whether `lang_name` and `plural_forms` for a newly-added language
match the fake's placeholder guesses (`lang_name` = the ISO code,
`plural_forms` = `["one","other"]`) or whether the real API fills in a
proper display name and locale-appropriate plural categories.

Observed: 2026-10-29: a project language's `lang_id` is the system ID (`en` 640, `fr` 673, `de` 666). Real names and plural forms come back (`French`, `["one","other"]`); the fake's ISO-code names are placeholders.## 6. Deleting the base language

Assumption: `DELETE /projects/{id}/languages/{base_language_id}` fails (the
project can't exist without its base language), matching
`lokalise_project_language`'s state-only removal of an imported base
language (it never calls DELETE on the base language).

```shell
curl -s -X DELETE -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/projects/<project_id>/languages/<base_language_id>"
```

Observed: 2026-10-29: **assumption wrong.** DELETE on the base language returns `200 {"language_deleted":true}`. The provider never deletes the base language, so nothing depends on this; the fake now allows it too.## 7. Duplicate language status code

Assumption: creating a language ISO that already exists on the project
returns an error (exact status/body unconfirmed; the provider never parses
it — duplicates are caught by listing first).

Observed (status code + body): 2026-10-29: **not an HTTP error.** `200` with `"languages": []` and `"errors": [{"message":"Language is already added to the project","code":208,"lang_iso":"de"}]`. The client now returns `errors[0]` as an `*APIError`.## 13a. Custom permission-template roles beyond the 5 built-ins

Assumption: the fake always seeds the same 5 built-in roles (1 Manager,
2 Developer, 3 Content creator, 4 Reviewer, 5 Translator) for any
`team_id`, with no per-team customization. A real team may have additional
custom roles created through the UI. Confirm `GET /teams/{team_id}/roles`
lists both the built-ins and any custom roles, and that
`lokalise_permission_template`'s lookup-by-name works correctly for a
custom role (not just the 5 built-ins).

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/teams/$TEAM_ID/roles"
```

```hcl
data "lokalise_permission_template" "custom" {
  team_id = <team_id>
  name    = "<a custom role name created by hand in the UI>"
}
```

Observed: 2026-10-29: the team tested has only the 5 built-in roles; no custom role to test.## 14. `GET /teams/{team_id}/roles` field names

Assumption: `{"roles":[{"id":N,"role":"Name", ...}, ...]}` — the ID field is
`id` (fixed from the OpenAPI example; previously assumed `role_id`, which
decoded every template ID as 0), the name field is `role` (feeds
`lokalise_permission_template`).

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/teams/$TEAM_ID/roles" | jq '.roles[0] | {id, role}'
```

Observed: 2026-10-29: as assumed (`id`, `role`).## 15. Group create/update response envelope and language ID space

Assumption: `POST .../groups` and `PUT .../groups/{group_id}` return
`{"team_id":.., "group":{...}}`, while `GET .../groups/{group_id}` returns
the group object unwrapped (fixed from the OpenAPI examples; previously all
three were assumed unwrapped, which decoded a created group's ID as 0 and
would create an orphan group on every apply). The client accepts either form
on every group endpoint. Group
`languages` on write use **system** language IDs from `GET /system/languages`
(not per-project language IDs), sent as
`"languages":{"reference":[ids],"contributable":[ids]}`.

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/system/languages"

curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"tf-manual-check-group","languages":{"reference":[640],"contributable":[640]},"admin_rights":["upload"]}' \
  "https://api.lokalise.com/api2/teams/$TEAM_ID/groups" | jq 'keys'

curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/teams/$TEAM_ID/groups/<group_id>" | jq 'keys'
```

Expect `["group","team_id"]` for the POST and the group's own keys
(`group_id`, `name`, `permissions`, ...) for the GET.

Observed: 2026-10-29: as assumed. POST/PUT keys are `group`, `team_id`; GET is unwrapped. System language IDs work.## 15a. `GET /system/languages` envelope and pagination

Assumption: the response is `{"languages": [{"lang_id", "lang_iso"}]}`,
either unpaginated or returning everything on page 1 — the client's
`listAll` helper tolerates a response with no `X-Pagination-*` headers, but
was never exercised against a real multi-page system-languages response.
Confirm the full catalogue (all ~400+ system languages) is actually
returned by a single call, or that `listAll`'s pagination handling correctly
follows `X-Pagination-*` headers if Lokalise does paginate this endpoint.

```shell
curl -sD - -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/system/languages" -o /dev/null | grep -i x-pagination

curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/system/languages" | jq '.languages | length'
```

Expect the `X-Pagination-*` headers to say the same total as `languages |
length` on this one call (i.e. everything came back on page 1), or for the
headers to be absent entirely (unpaginated).

Observed: 2026-10-29: **paginated**: `x-pagination-total-count: 734`, 100 per page, 8 pages. `listAll` follows `x-pagination-page-count` correctly (groups with `en`/`fr` were created through Terraform).## 16. Group GET `permissions.languages` shape

Assumption: `GET .../groups/{id}` nests a flat array
`[{"lang_id", "lang_iso", "lang_name", "is_writable"}]` under
`permissions.languages`, with `is_writable` expressing reference (`false`)
vs. contributable (`true`). This matches the OpenAPI schema and examples,
but the mapping of `is_writable` to the request's `reference`/`contributable`
split is inferred; confirm a group created with `reference:[640]` reads back
with `is_writable: false` for 640.

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/teams/$TEAM_ID/groups/<group_id>"
```

Observed: 2026-10-29: a language in both `reference` and `contributable` is stored once, `is_writable: true`. After an update with `reference:[640], contributable:[]`, 640 reads back `is_writable: false`.## 17. Group with no languages, `admin_rights`-only

Assumption: a group create/update with no languages sends
`"languages":{"reference":[],"contributable":[]}` (empty arrays, never
`null`; the OpenAPI request examples use `{}`). Confirm the real API
accepts this for an `admin_rights`-only group (no language grants at all),
and that such a group round-trips with no diff on a second
`terraform plan`.

```hcl
resource "lokalise_team_user_group" "admin_only" {
  team_id      = <team_id>
  name         = "tf-manual-check-admin-only"
  admin_rights = ["upload"]
}
```

Apply, then `terraform plan` again with no config change.

Observed: 2026-10-29: **rejected.** Empty or missing `reference` → `400 Invalid languages.reference parameter`; missing `contributable` → `400 Invalid languages.contributable parameter`. `reference:[640], contributable:[]` is accepted on create but stores **no** languages (on update it stores 640 as reference). Fixed: `languages.contributable` is now required with at least one entry, and the client sends the contributable languages as `reference` when `reference` is empty.## 18. `is_admin`/`is_reviewer` on group create/update

Assumption: these are deprecated boolean fields the create/update endpoints
still require; the client always sends `false` for both. Confirm the API
accepts `false,false` for every group regardless of `admin_rights`/`role_id`.

Observed: 2026-10-29: `false,false` is accepted, but a group created with `admin_rights` reads back `is_admin: true`. The provider doesn't expose `is_admin`.## 19. Group member/project add-remove envelope

Confirmed against the OpenAPI examples (worth a live sanity check):
`PUT .../groups/{id}/members/add|remove` and
`PUT .../groups/{id}/projects/add|remove` return
`{"team_id": <int>, "group": <Group>}` — the same envelope as group create
and update (see #15); only group GET is unwrapped. The client discards these
bodies.

```shell
curl -s -X PUT -H "X-Api-Token: $LOKALISE_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"users":[<user_id>]}' \
  "https://api.lokalise.com/api2/teams/$TEAM_ID/groups/<group_id>/members/add"
```

Observed: 2026-10-29: as assumed; every add/remove returns `{team_id, group}`.## 21. `GET /teams/{team_id}/users` role field

Confirmed against the OpenAPI spec (not a guess), worth a live check: `role`
is a plain string (e.g. `"owner"`, `"member"`), not a structured object.

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/teams/$TEAM_ID/users"
```

Observed: 2026-10-29: as assumed (`"member"`).## 22. 429 behavior under load

Assumption: the client's shared token-bucket rate limiter (≤6 req/s) plus
exponential-backoff-with-jitter retry on 429 (all methods) and 5xx/network
errors (GET/PUT/DELETE only; see #29) is enough to make a many-resource
apply succeed without a hard failure.

```shell
terraform apply -parallelism=10
```

against a config with at least 10-15 resources (project + several languages
+ groups, a member and a group-project assignment). Watch for any `429` that surfaces as a Terraform
error instead of being retried transparently.

Observed: 2026-10-29: 13 resources at `-parallelism=10` applied in ~2s with no 429 surfaced.## 23. Import of a real existing project yields an empty plan

Pick (or hand-create in the UI) an existing project not managed by
Terraform.

```shell
terraform import lokalise_project.imported <project_id>
terraform plan
```

Expected: no diff, once the config's `name`/`description`/`base_language_iso`
match the imported state, and `deletion_protection = true` needs no config
line to avoid a diff.

Observed:

## 24. Error body shape

Assumption: `{"error":{"message":"...","code":N}}` on every error response,
optionally with `"details"`, used by `client.IsNotFound` and surfaced error
messages (`details`, when present, is appended as compact JSON). Trigger a
validation error and confirm what `details` looks like:

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"languages":[{"lang_iso":"not-a-language"}]}' \
  "https://api.lokalise.com/api2/projects/<project_id>/languages"
```

Observed: 2026-10-29: language create reports this in a 200 body, not an error body: `{"errors":[{"message":"`lang_iso` parameter should be in a scope of system languages","code":400,"lang_iso":"not-a-language"}]}`. Other endpoints' 400s use `{"error":{"message","code"}}` with no `details` seen.## 25. Group `role_id` location

Assumption: the OpenAPI `TeamUserGroups` schema puts `role_id` at the top
level of a group, but every example nests it under `permissions.role_id`.
The client reads `permissions.role_id` and falls back to the top level; the
fake emits it under `permissions`. Confirm where the real API puts it (and
that it is present at all for a group created with `role_id`):

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/teams/$TEAM_ID/groups/<group_id>" | jq '{top: .role_id, nested: .permissions.role_id}'
```

Observed: 2026-10-29: nested under `permissions.role_id`; `null` when no role.## 27. `members/add` integer vs string user IDs

Assumption: `PUT .../groups/{id}/members/add|remove` take
`{"users":[<int>, ...]}` (integers, as the OpenAPI example shows) and
`members` reads back as integers. Confirm the API does not require or
return strings (e.g. `["22212"]`), which would break the client's
`[]int64` decode and `lokalise_team_user_group_member`'s membership check.

```shell
curl -s -X PUT -H "X-Api-Token: $LOKALISE_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"users":[<user_id>]}' \
  "https://api.lokalise.com/api2/teams/$TEAM_ID/groups/<group_id>/members/add" | jq '.group.members'
```

Observed: 2026-10-29: `members` are integers; `projects` are project-ID strings.## 28. `{}` empty arrays and integer `is_writable`

Assumption: the OpenAPI examples serialize empty arrays as `{}`
(`"admin_rights": {}`, `"members": {}`, `"projects": {}`,
`"languages": {}`) and list endpoints return `"is_writable": 1` (an
integer). The client tolerates `[]`, `{}`, `null` (and index-keyed objects)
for these arrays and `true/false/0/1` for `is_writable`; the fake emits the
`{}` and integer forms. Confirm on a live group with no members/projects and
on the contributor list:

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/teams/$TEAM_ID/groups/<group_id>" | jq '{members, projects, admin_rights: .permissions.admin_rights}'

curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/projects/<project_id>/contributors" | jq '.contributors[0].languages[0].is_writable'
```

Note any other encoding (e.g. `"1"`, `"true"`) — the client accepts those
too, but anything else fails to decode.

Observed: 2026-10-29: empty arrays come back as `[]`, not `{}`; contributor `is_writable` is a boolean. A group's `admin_rights` is never empty: Lokalise adds `branches_main_modify` to every group, with or without `role_id`. Fixed: `lokalise_team_user_group` keeps the configured `admin_rights` when the API's set contains all of them.## 29. POST is not retried on 5xx or network errors

Behavior (not an assumption): the client retries a `POST` only on `429`. A
`5xx` (e.g. a `502`/`504` from a gateway) or a dropped connection on a
create is returned as an error immediately, because the create may have
succeeded server-side and retrying would create a duplicate project, group,
language or group. GET/PUT/DELETE still retry `5xx` and network
errors.

If `terraform apply` fails on a create with a gateway/network error:

1. Check the Lokalise UI (or list the relevant endpoint) for the
   project/group/language the apply tried to create.
2. If it exists, `terraform import` it (each resource's docs give the import
   ID format) instead of re-applying, which would fail with "already exists"
   (languages, group members/projects) or create a duplicate
   (projects, groups).
3. If it does not exist, re-run `terraform apply`.

Observed (any gateway errors seen on create): 2026-10-29: none.

---

## Deferred with `lokalise_contributor`

`lokalise_contributor` was removed from the provider: creating a contributor
needs a second Lokalise user who is not the token owner. It lives on the
`contributor-resource` branch. Run these items before bringing it back.
The client's contributor methods are still in `internal/client` and the
fake.

### 8. Contributor response shape

Assumption: a contributor object has `user_id`, `email`, `fullname`,
`role_id` (nullable), `admin_rights` (`[]string`, or `{}` when empty),
`languages` (`[]{lang_iso, is_writable}`). Create is a bulk POST
`{"contributors":[{...}]}` returning the created contributor under a
**singular** key, `{"project_id":.., "contributor":{...}}`; the client also
accepts a plural `"contributors"` array. `GET` and `PUT
.../contributors/{user_id}` return `{"project_id":.., "contributor":{...}}`.
(All three envelopes fixed from the OpenAPI examples; previously assumed
plural on POST and unwrapped on GET/PUT.) Update is a `PUT` with a flat body
of `role_id`/`admin_rights`/`languages` only (no `email`/`fullname`).

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" -H 'Content-Type: application/json' \
  -d '{"contributors":[{"email":"you@example.com","role_id":5,"languages":[{"lang_iso":"fr","is_writable":true}]}]}' \
  "https://api.lokalise.com/api2/projects/<project_id>/contributors" | jq 'keys'

curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/projects/<project_id>/contributors/<user_id>" | jq 'keys'
```

Expect `["contributor","project_id"]` for both.

Observed:

### 9. Contributor GET returns `role_id`

Assumption: `role_id` is present (possibly `null`) on every contributor GET,
not omitted when a contributor was created with `admin_rights` instead of a
role.

Observed:

### 10. Duplicate contributor email — case-insensitivity

Assumption: the real API de-duplicates contributor emails case-insensitively
(the provider's own list-before-create check does `strings.EqualFold`, per
`TestContributor_` case-insensitive-email coverage from Task 5). Confirm that
adding `USER@Example.com` when `user@example.com` is already a contributor
is rejected or treated as the same contributor by Lokalise, not created as a
second one.

Also note whether the API returns the email as sent or normalized (e.g.
lowercased): `lokalise_contributor` keeps the configured email whenever the
API's matches it case-insensitively, and never reads `fullname` back after
create (the API reports an existing user's own name), so neither should
cause a diff.

Observed:

### 11. Duplicate contributor status code

Assumption: creating a contributor whose email is already on the project
returns an error (status/body unconfirmed; not parsed by the provider).

Observed (status code + body):

### 12. `admin_rights` normalization/order

Assumption: `admin_rights` values round-trip as configured; if the server
reorders the array or normalizes casing/spelling, repeated applies with the
same config must not show a diff (the provider should compare as sets, not
ordered lists — confirm this is actually true against the live API, not just
the fake).

```hcl
resource "lokalise_contributor" "check" {
  project_id   = lokalise_project.check.id
  email        = "you@example.com"
  admin_rights = ["upload", "download"]
  languages    = [{ lang_iso = "en", is_writable = true }]
}
```

Apply, then run `terraform plan` again with no config change.

Observed:

### 13. Does deleting a contributor remove group-granted access?

Assumption: unknown and not modeled. If a user has project access only
because their group was assigned to the project (see #16), does
`DELETE .../contributors/{user_id}` remove them, or does the group grant
re-add them on the next sync? This affects whether `lokalise_contributor`
and `lokalise_team_user_group_project` can safely coexist on the same
project for the same user.

Observed:

### 20. Does assigning a group to a project create contributors?

Assumption: unknown. If `lokalise_team_user_group_project` (which calls
`.../groups/{id}/projects/add`) causes Lokalise to implicitly add each group
member as a contributor on that project, then a `lokalise_contributor`
resource for the same user on the same project would collide with (or
duplicate) group-granted access. Check whether `GET
/projects/{id}/contributors` lists group members after assigning the group,
and whether those members appear with a `role_id`/`admin_rights` of their
own or only through group membership.

Observed:

### 26. Contributors with admin/all-language roles returning extra languages

Assumption: a contributor's `languages` read back exactly as configured. If
Lokalise grants admins or certain roles (e.g. Manager, or a template with
`doesEnableAllReadOnlyLanguages`) every project language automatically,
`GET .../contributors/{user_id}` may list languages that were never
configured, and `lokalise_contributor` will show a perpetual diff on
`languages`. Create a contributor with `role_id = 1` and one language on a
project with several languages, then:

```shell
curl -s -H "X-Api-Token: $LOKALISE_API_TOKEN" \
  "https://api.lokalise.com/api2/projects/<project_id>/contributors/<user_id>" | jq '.contributor.languages'
```

and run `terraform plan` with no config change.

Observed:
