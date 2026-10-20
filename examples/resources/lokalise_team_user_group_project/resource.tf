resource "lokalise_project" "marketing" {
  name                = "Marketing site"
  base_language_iso   = "en"
  deletion_protection = false
}

resource "lokalise_team_user_group" "french_translators" {
  team_id = 123456
  name    = "French translators"
  role_id = 5

  languages = {
    reference     = ["en"]
    contributable = ["fr"]
  }
}

resource "lokalise_team_user_group_project" "marketing" {
  team_id    = 123456
  group_id   = lokalise_team_user_group.french_translators.group_id
  project_id = lokalise_project.marketing.id
}
