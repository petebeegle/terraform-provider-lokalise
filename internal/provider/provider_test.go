// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testProviderFactories is used to instantiate a provider during testing.
// The factory function is called for each Terraform CLI command to create a
// provider server that the CLI can connect to and interact with.
var testProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"lokalise": providerserver.NewProtocol6WithError(New("test")()),
}

// providerConfig returns a provider configuration block pointed at apiURL,
// for use with lokalisefake in resource/data source tests.
func providerConfig(apiURL string) string {
	return fmt.Sprintf(`
provider "lokalise" {
  token   = "test-token"
  api_url = %q
}
`, apiURL)
}

func TestProvider_SchemaValid(t *testing.T) {
	ctx := context.Background()

	p := New("test")()
	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, schemaResp)

	diags := schemaResp.Schema.ValidateImplementation(ctx)
	if diags.HasError() {
		t.Fatalf("schema validation errors: %v", diags.Errors())
	}

	tokenAttr, ok := schemaResp.Schema.Attributes["token"]
	if !ok {
		t.Fatal("schema missing \"token\" attribute")
	}
	if !tokenAttr.IsSensitive() {
		t.Fatal("\"token\" attribute must be sensitive")
	}
}
