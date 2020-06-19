package fastly

import (
	"fmt"
	"log"
	"reflect"
	"testing"

	gofastly "github.com/fastly/go-fastly/fastly"
	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-sdk/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
)

func TestResourceFastlyFlattenGooglePubSub(t *testing.T) {
	cases := []struct {
		remote []*gofastly.Pubsub
		local  []map[string]interface{}
	}{
		{
			remote: []*gofastly.Pubsub{
				{
					Version:           1,
					Name:              "googlepubsub-endpoint",
					User:              "user",
					SecretKey:         privateKey(),
					ProjectID:         "project-id",
					Topic:             "topic",
					ResponseCondition: "response_condition",
					Format:            `%a %l %u %t %m %U%q %H %>s %b %T`,
					FormatVersion:     2,
					Placement:         "none",
				},
			},
			local: []map[string]interface{}{
				{
					"name":               "googlepubsub-endpoint",
					"user":               "user",
					"secret_key":         privateKey(),
					"project_id":         "project-id",
					"topic":              "topic",
					"response_condition": "response_condition",
					"format":             `%a %l %u %t %m %U%q %H %>s %b %T`,
					"placement":          "none",
					"format_version":     uint(2),
				},
			},
		},
	}

	for _, c := range cases {
		out := flattenGooglePubSub(c.remote)
		if !reflect.DeepEqual(out, c.local) {
			t.Fatalf("Error matching:\nexpected: %#v\n got: %#v", c.local, out)
		}
	}

}

func TestAccFastlyServiceV1_googlepubsublogging_basic(t *testing.T) {
	var service gofastly.ServiceDetail
	name := fmt.Sprintf("tf-test-%s", acctest.RandString(10))
	domain := fmt.Sprintf("fastly-test.%s.com", name)

	log1 := gofastly.Pubsub{
		Version:           1,
		Name:              "googlepubsublogger",
		User:              "user",
		SecretKey:         privateKey() + "\n", // The '\n' is necessary becaue of the heredocs (i.e., EOF) in the config below.
		ProjectID:         "project-id",
		Topic:             "topic",
		ResponseCondition: "response_condition_test",
		Format:            `%a %l %u %t %m %U%q %H %>s %b %T`,
		FormatVersion:     2,
		Placement:         "none",
	}

	log1_after_update := gofastly.Pubsub{
		Version:           1,
		Name:              "googlepubsublogger",
		User:              "newuser",
		SecretKey:         privateKey() + "\n", // The '\n' is necessary becaue of the heredocs (i.e., EOF) in the config below.
		ProjectID:         "new-project-id",
		Topic:             "newtopic",
		ResponseCondition: "response_condition_test",
		Format:            `%a %l %u %t %m %U%q %H %>s %b %T`,
		FormatVersion:     2,
		Placement:         "waf_debug",
	}

	log2 := gofastly.Pubsub{
		Version:           1,
		Name:              "googlepubsublogger2",
		User:              "user2",
		SecretKey:         privateKey() + "\n", // The '\n' is necessary becaue of the heredocs (i.e., EOF) in the config below.
		ProjectID:         "project-id",
		Topic:             "topicb",
		ResponseCondition: "response_condition_test",
		Format:            `%a %l %u %t %m %U%q %H %>s %b %T`,
		FormatVersion:     2,
		Placement:         "none",
	}

	resource.Test(t, resource.TestCase{
		PreCheck:     func() { testAccPreCheck(t) },
		Providers:    testAccProviders,
		CheckDestroy: testAccCheckServiceV1Destroy,
		Steps: []resource.TestStep{
			{
				Config: testAccServiceV1GooglePubSubConfig(name, domain),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServiceV1Exists("fastly_service_v1.foo", &service),
					testAccCheckFastlyServiceV1GooglePubSubAttributes(&service, []*gofastly.Pubsub{&log1}),
					resource.TestCheckResourceAttr(
						"fastly_service_v1.foo", "name", name),
					resource.TestCheckResourceAttr(
						"fastly_service_v1.foo", "logging_googlepubsub.#", "1"),
				),
			},

			{
				Config: testAccServiceV1GooglePubSubConfig_update(name, domain),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServiceV1Exists("fastly_service_v1.foo", &service),
					testAccCheckFastlyServiceV1GooglePubSubAttributes(&service, []*gofastly.Pubsub{&log1_after_update, &log2}),
					resource.TestCheckResourceAttr(
						"fastly_service_v1.foo", "name", name),
					resource.TestCheckResourceAttr(
						"fastly_service_v1.foo", "logging_googlepubsub.#", "2"),
				),
			},
		},
	})
}

func testAccCheckFastlyServiceV1GooglePubSubAttributes(service *gofastly.ServiceDetail, googlepubsub []*gofastly.Pubsub) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		conn := testAccProvider.Meta().(*FastlyClient).conn
		googlepubsubList, err := conn.ListPubsubs(&gofastly.ListPubsubsInput{
			Service: service.ID,
			Version: service.ActiveVersion.Number,
		})

		if err != nil {
			return fmt.Errorf("[ERR] Error looking up Google Cloud Pub/Sub Logging for (%s), version (%d): %s", service.Name, service.ActiveVersion.Number, err)
		}

		if len(googlepubsubList) != len(googlepubsub) {
			return fmt.Errorf("Google Cloud Pub/Sub List count mismatch, expected (%d), got (%d)", len(googlepubsub), len(googlepubsubList))
		}

		log.Printf("[DEBUG] googlepubsubList = %#v\n", googlepubsubList)

		var found int
		for _, s := range googlepubsub {
			for _, sl := range googlepubsubList {
				if s.Name == sl.Name {
					// we don't know these things ahead of time, so populate them now
					s.ServiceID = service.ID
					s.Version = service.ActiveVersion.Number
					// We don't track these, so clear them out because we also wont know
					// these ahead of time
					sl.CreatedAt = nil
					sl.UpdatedAt = nil
					if diff := cmp.Diff(s, sl); diff != "" {
						return fmt.Errorf("Bad match Google Cloud Pub/Sub logging match: %s", diff)
					}
					found++
				}
			}
		}

		if found != len(googlepubsub) {
			return fmt.Errorf("Error matching Google Cloud Pub/Sub Logging rules")
		}

		return nil
	}
}

func testAccServiceV1GooglePubSubConfig(name string, domain string) string {
	return fmt.Sprintf(`
resource "fastly_service_v1" "foo" {
	name = "%s"

	domain {
		name    = "%s"
		comment = "tf-googlepubsub-logging"
	}

	backend {
		address = "aws.amazon.com"
		name    = "amazon docs"
	}

	condition {
    name      = "response_condition_test"
    type      = "RESPONSE"
    priority  = 8
    statement = "resp.status == 418"
  }

	logging_googlepubsub {
		name               = "googlepubsublogger"
		user               = "user"
		secret_key         = <<EOF
`+privateKey()+`
EOF
		project_id         = "project-id"
	  topic  						 = "topic"
		response_condition = "response_condition_test"
		format             = "%%a %%l %%u %%t %%m %%U%%q %%H %%>s %%b %%T"
		format_version     = 2
		placement          = "none"
	}

	force_destroy = true
}
`, name, domain)
}

func testAccServiceV1GooglePubSubConfig_update(name, domain string) string {
	return fmt.Sprintf(`
resource "fastly_service_v1" "foo" {
	name = "%s"

	domain {
		name    = "%s"
		comment = "tf-testing-domain"
	}

	backend {
		address = "aws.amazon.com"
		name    = "amazon docs"
	}

	condition {
    name      = "response_condition_test"
    type      = "RESPONSE"
    priority  = 8
    statement = "resp.status == 418"
  }

	logging_googlepubsub {
		name               = "googlepubsublogger"
		user               = "newuser"
		secret_key         = <<EOF
`+privateKey()+`
EOF
		project_id         = "new-project-id"
	  topic  						 = "newtopic"
		response_condition = "response_condition_test"
		format             = "%%a %%l %%u %%t %%m %%U%%q %%H %%>s %%b %%T"
		format_version     = 2
		placement          = "waf_debug"
	}

	logging_googlepubsub {
		name               = "googlepubsublogger2"
		user               = "user2"
		secret_key         = <<EOF
`+privateKey()+`
EOF
		project_id         = "project-id"
	  topic  						 = "topicb"
		response_condition = "response_condition_test"
		format             = "%%a %%l %%u %%t %%m %%U%%q %%H %%>s %%b %%T"
		format_version     = 2
		placement          = "none"
	}

	force_destroy = true
}`, name, domain)
}