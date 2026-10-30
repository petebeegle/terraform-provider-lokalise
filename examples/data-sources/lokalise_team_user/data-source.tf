data "lokalise_team_user" "jane" {
  team_id = 123456
  email   = "jane@example.com"
}

resource "lokalise_team_user_group_member" "jane" {
  team_id  = 123456
  group_id = lokalise_team_user_group.french_translators.group_id
  user_id  = data.lokalise_team_user.jane.user_id
}
