resource "lokalise_project_language" "french" {
  project_id = lokalise_project.example.id
  lang_iso   = "fr"

  # custom_name and plural_forms default to Lokalise's values for lang_iso.
  # custom_name  = "French (FR)"
  # plural_forms = ["one", "other"]

  # deletion_protection defaults to true: `terraform destroy` fails instead of
  # deleting the language. Set it to false and apply before destroying or
  # replacing it. Has no effect on the project's base language, which this
  # resource never deletes: removing it only clears Terraform state.
  # deletion_protection = false
}
