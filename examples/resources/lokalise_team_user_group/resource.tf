data "lokalise_permission_template" "translator" {
  team_id = 123456
  name    = "Translator"
}

resource "lokalise_team_user_group" "french_translators" {
  team_id = 123456
  name    = "French translators"

  # Exactly one of role_id or admin_rights must be set.
  role_id = data.lokalise_permission_template.translator.role_id
  # admin_rights = ["upload", "download"]

  languages = {
    reference     = ["en"]
    contributable = ["fr"]
  }
}

# Members and project assignments are managed by
# lokalise_team_user_group_member and lokalise_team_user_group_project; this
# resource only manages the group's own name/role/languages.
