data "lokalise_permission_template" "translator" {
  team_id = 123456
  name    = "Translator"
}

resource "lokalise_team_user_group" "french_translators" {
  team_id = 123456
  name    = "French translators"
  role_id = data.lokalise_permission_template.translator.role_id

  languages = {
    reference     = ["en"]
    contributable = ["fr"]
  }
}
