resource "lokalise_project" "example" {
  name              = "My App"
  description       = "iOS + Android strings"
  base_language_iso = "en"

  # deletion_protection defaults to true: `terraform destroy` fails instead of
  # deleting the project (and all its translations). Set it to false and apply
  # before destroying or replacing the project.
  # deletion_protection = false
}
