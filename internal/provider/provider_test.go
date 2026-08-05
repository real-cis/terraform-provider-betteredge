// Copyright (c) real-cis <info@real-cis.com>
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"betteredge": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	if os.Getenv("BETTEREDGE_PLATFORM_URL") == "" {
		t.Fatal("BETTEREDGE_PLATFORM_URL must be set for acceptance tests")
	}
	if os.Getenv("BETTEREDGE_API_TOKEN") == "" {
		t.Fatal("BETTEREDGE_API_TOKEN must be set for acceptance tests")
	}
}
