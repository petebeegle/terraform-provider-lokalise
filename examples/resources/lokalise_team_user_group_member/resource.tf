data "lokalise_team_user" "jane" {
  team_id = 123456
  email   = "jane@example.com"
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

resource "lokalise_team_user_group_member" "jane" {
  team_id  = 123456
  group_id = lokalise_team_user_group.french_translators.group_id
  user_id  = data.lokalise_team_user.jane.user_id
}
