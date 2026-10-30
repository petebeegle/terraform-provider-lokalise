terraform {
  required_providers {
    lokalise = {
      source = "petebeegle/lokalise"
    }
  }
}

# Reads the API token from the LOKALISE_API_TOKEN environment variable.
provider "lokalise" {}
