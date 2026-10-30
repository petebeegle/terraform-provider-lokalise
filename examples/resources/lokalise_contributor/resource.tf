data "lokalise_permission_template" "translator" {
  team_id = lokalise_project.example.team_id
  name    = "Translator"
}

resource "lokalise_contributor" "french_translator" {
  project_id = lokalise_project.example.id
  email      = "translator@example.com"

  # fullname is only used when inviting a brand-new Lokalise user; it has no
  # effect for an email that is already a Lokalise user.
  # fullname = "Jane Doe"

  # Exactly one of role_id or admin_rights must be set.
  role_id = data.lokalise_permission_template.translator.role_id
  # admin_rights = ["upload", "download"]

  languages = [
    { lang_iso = "fr", is_writable = true },
  ]
}

# WARNING: the API token's own user is a contributor on every project it can
# see. Managing that contributor with this resource is allowed, but removing
# it (destroying the resource, or narrowing its languages/rights) can lock
# the token out of the project.
