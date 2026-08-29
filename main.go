package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"terraform-provider-ai/internal/delivery"
)

// version is set at build time via ldflags:
//
//	-X main.version=<semver>
//
// e.g.: go build -ldflags "-X main.version=0.2.0"
var version = "dev"

func main() {
	err := providerserver.Serve(context.Background(),
		func() provider.Provider { return delivery.NewProvider(delivery.WithVersion(version)) },
		providerserver.ServeOpts{
			Address: "registry.example.com/ai/ai",
		},
	)
	if err != nil {
		log.Fatal(err)
	}
}
