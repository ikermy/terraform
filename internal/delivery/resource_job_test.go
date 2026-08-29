package delivery

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-ai/api"
)

func TestAccJob_CreateUpdateDelete(t *testing.T) {
	srv := httptest.NewServer(api.NewServer())
	t.Cleanup(srv.Close)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{
				Config: testAccJobConfig("demo-job", "echo hello", 5),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("aiprovider_job.demo", "name", "demo-job"),
					resource.TestCheckResourceAttr("aiprovider_job.demo", "command", "echo hello"),
					resource.TestCheckResourceAttr("aiprovider_job.demo", "priority", "5"),
					resource.TestCheckResourceAttrSet("aiprovider_job.demo", "id"),
					resource.TestCheckResourceAttrSet("aiprovider_job.demo", "status"),
				),
			},
			{
				Config: testAccJobConfig("demo-job", "echo world", 9),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("aiprovider_job.demo", "command", "echo world"),
					resource.TestCheckResourceAttr("aiprovider_job.demo", "priority", "9"),
				),
			},
		},
	})
}

func TestAccJob_Import(t *testing.T) {
	srv := httptest.NewServer(api.NewServer())
	t.Cleanup(srv.Close)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(srv.URL),
		Steps: []resource.TestStep{
			{
				Config: testAccJobConfig("import-job", "echo hi", 1),
			},
			{
				ResourceName:      "aiprovider_job.demo",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccJobConfig(name, command string, priority int) string {
	return fmt.Sprintf(`
provider "aiprovider" {}

resource "aiprovider_job" "demo" {
  name     = %q
  command  = %q
  priority = %d
}
`, name, command, priority)
}
