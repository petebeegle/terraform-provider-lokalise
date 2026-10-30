# Terraform Provider for Lokalise

A Terraform provider for managing [Lokalise](https://lokalise.com) projects,
project languages, and team user groups (permissions, members, project
assignments) — the setup work done by hand in the Lokalise UI today.
Project access is granted through groups: a group's role and languages,
its members, and the projects assigned to it.

## What this provider manages

- `lokalise_project` — a project (name, description, team, type, base
  language).
- `lokalise_project_language` — a language added to a project.
- `lokalise_team_user_group` — a team user group's own fields (name, role /
  `admin_rights`, language grants).
- `lokalise_team_user_group_member` — one user's membership in a group.
- `lokalise_team_user_group_project` — one project assigned to a group.
- `lokalise_permission_template` (data source) — look up a team's built-in
  or custom role by name, to avoid hard-coding `role_id`.
- `lokalise_team_user` (data source) — look up a team user by email, to feed
  `lokalise_team_user_group_member`.

Every resource is importable, so existing hand-made projects can be adopted
into Terraform.

### What it does not manage (non-goals)

- Automations and apps/integrations — no public Lokalise API.
- Project `settings` (reviewing, branching, per-platform key names) — the
  API only exposes these read-only.
- Keys, translations, files, screenshots, glossary — content, not setup.
- Webhooks.
- Per-project contributors. Grant access through a group instead
  (`lokalise_team_user_group` + `_member` + `_project`); one-off contributors
  stay in the UI. A `lokalise_contributor` resource exists on the
  `contributor-resource` branch but is unverified against the live API,
  which needs a second Lokalise user to test.
- Creating or inviting team users directly — no such API exists. A user
  must already be on the team before `lokalise_team_user_group_member` can
  add them to a group.

### Non-authoritative

This provider is **non-authoritative**: no resource removes anything it did
not create itself. One resource instance manages exactly one item (one
language, one group member, one group-project assignment).
Declaring a `lokalise_team_user_group_member` for Alice never removes other
members the UI or another team added by hand; destroying that resource
removes only Alice. If you want a resource gone, you must
explicitly destroy it — nothing is pruned implicitly by omission from
config.

### `deletion_protection`

`lokalise_project` and `lokalise_project_language` both have a
`deletion_protection` attribute, `optional`, computed, **default `true`**.
While `true`, `terraform destroy` (or a replace that would destroy the
resource) fails with an error and makes no API call — it does not delete
the project or language. Set it to `false` and `apply` that change first,
then destroy. Imported resources always come in with
`deletion_protection = true`, so adopting an existing project can never
accidentally make it destroyable without an explicit, separate step.

This exists because deleting a project or a language deletes its
translation data, which is usually the most expensive mistake a
`terraform destroy` can make.

### Importing existing projects

Every resource supports `terraform import`:

```shell
terraform import lokalise_project.example <project_id>
terraform import lokalise_project_language.example <project_id>:<lang_iso>
terraform import lokalise_team_user_group.example <team_id>:<group_id>
terraform import lokalise_team_user_group_member.example <team_id>:<group_id>:<user_id>
terraform import lokalise_team_user_group_project.example <team_id>:<group_id>:<project_id>
```

See each resource's generated doc under `docs/resources/` for its exact
import ID and an example. After importing a project or a project language,
`deletion_protection` comes in as `true`; a config that omits the attribute
produces an empty plan.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.24 (for building from source)
- A Lokalise API token (`LOKALISE_API_TOKEN`), team Admin to create projects

## Configuration

```hcl
terraform {
  required_providers {
    lokalise = {
      source = "petebeegle/lokalise"
    }
  }
}

provider "lokalise" {
  # token can also be set via the LOKALISE_API_TOKEN environment variable
  token = "example-token"
}
```

- `token` (sensitive, optional): falls back to the `LOKALISE_API_TOKEN`
  environment variable. Configuration fails at `terraform plan`/`apply` if
  neither is set.
- `api_url` (optional): defaults to `https://api.lokalise.com/api2`.

## Building and running locally (`dev_overrides`)

This provider isn't published to a registry yet. To use a locally built
binary without `terraform init`, use Terraform's
[`dev_overrides`](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers):

```shell
go install .
```

Add to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "petebeegle/lokalise" = "<GOBIN>"
  }

  direct {}
}
```

Replace `<GOBIN>` with the output of `go env GOBIN` (or `$(go env GOPATH)/bin`
if `GOBIN` is unset). With `dev_overrides` active, Terraform uses this local
binary for every `lokalise` provider block and skips `terraform init`'s
provider installation for it — remove the block to go back to a normal
registry install.

## Developing the provider

```shell
go build ./...
go vet ./...
make test       # go test ./... — unit tests only, against an in-process
                # fake Lokalise server; never calls the real API
make generate   # regenerates docs/ via tfplugindocs and runs
                # terraform fmt on examples/
make lint       # golangci-lint
```

There is no `TF_ACC` acceptance-test suite in v1; real projects are never
touched by this repo's tests. See
[`docs/manual-verification.md`](docs/manual-verification.md) for the
checklist to run by hand against a real Lokalise team before trusting the
provider's assumed API wire shapes.
