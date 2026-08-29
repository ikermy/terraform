package delivery

import (
	"fmt"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-ai/api"
)

func testAccProtoV6ProviderFactories(endpoint string) map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"aiprovider": providerserver.NewProtocol6WithError(NewProvider(WithDefaultEndpoint(endpoint))),
	}
}

func TestAccCluster_CreateUpdateDelete(t *testing.T) {
	srv := httptest.NewServer(api.NewServer())
	t.Cleanup(srv.Close)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{
				Config: testAccClusterConfig("demo", 3, "gpt-mini"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccClusterExists("aiprovider_cluster.demo"),
					resource.TestCheckResourceAttr("aiprovider_cluster.demo", "name", "demo"),
					resource.TestCheckResourceAttr("aiprovider_cluster.demo", "replicas", "3"),
					resource.TestCheckResourceAttr("aiprovider_cluster.demo", "model", "gpt-mini"),
				),
			},
			{
				Config: testAccClusterConfig("demo", 5, "gpt-large"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("aiprovider_cluster.demo", "replicas", "5"),
					resource.TestCheckResourceAttr("aiprovider_cluster.demo", "model", "gpt-large"),
				),
			},
			{
				Config:   testAccClusterConfig("demo", 5, "gpt-large"),
				PlanOnly: true,
			},
		},
	})
}

func TestAccCluster_InvalidEndpoint(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories("http://localhost:0"),
		Steps: []resource.TestStep{
			{
				Config:      testAccClusterConfig("demo", 1, "gpt-mini"),
				ExpectError: regexp.MustCompile(`Create failed`),
			},
		},
	})
}

func TestAccCluster_Import(t *testing.T) {
	srv := httptest.NewServer(api.NewServer())
	t.Cleanup(srv.Close)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{
				Config: testAccClusterConfig("import-me", 4, "gpt-mini"),
			},
			{
				ResourceName:      "aiprovider_cluster.demo",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccDataSource_Cluster(t *testing.T) {
	srv := httptest.NewServer(api.NewServer())
	t.Cleanup(srv.Close)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "aiprovider" {}

resource "aiprovider_cluster" "demo" {
  name     = "ds-demo"
  replicas = 2
  model    = "gpt-mini"
}

data "aiprovider_cluster" "by_id" {
  id = aiprovider_cluster.demo.id
}
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccClusterExists("aiprovider_cluster.demo"),
					resource.TestCheckResourceAttr("data.aiprovider_cluster.by_id", "name", "ds-demo"),
					resource.TestCheckResourceAttr("data.aiprovider_cluster.by_id", "replicas", "2"),
					resource.TestCheckResourceAttr("data.aiprovider_cluster.by_id", "model", "gpt-mini"),
				),
			},
		},
	})
}

func testAccClusterConfig(name string, replicas int, model string) string {
	return fmt.Sprintf(`
provider "aiprovider" {}

resource "aiprovider_cluster" "demo" {
  name     = %q
  replicas = %d
  model    = %q
}
`, name, replicas, model)
}

func testAccClusterExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}
		if rs.Primary.ID == "" {
			return fmt.Errorf("resource %s has empty id", resourceName)
		}
		return nil
	}
}
