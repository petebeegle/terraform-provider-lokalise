data "lokalise_permission_template" "translator" {
  team_id = lokalise_project.example.team_id
  name    = "Translator"
}

resource "lokalise_contributor" "french_translator" {
  project_id = lokalise_project.example.id
  email      = "translator@example.com"
  role_id    = data.lokalise_permission_template.translator.role_id

  languages = [
    { lang_iso = "fr", is_writable = true },
  ]
}
