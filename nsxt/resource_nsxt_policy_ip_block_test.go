/* Copyright © 2019 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: MPL-2.0 */

package nsxt

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"

	"github.com/vmware/terraform-provider-nsxt/api/infra"
)

func TestAccResourceNsxtPolicyIPBlock_minimal(t *testing.T) {
	name := getAccTestResourceName()
	testResourceName := "nsxt_policy_ip_block.test"
	cidr := "192.168.1.0/24"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:  func() { testAccOnlyLocalManager(t); testAccPreCheck(t) },
		Providers: testAccProviders,
		CheckDestroy: func(state *terraform.State) error {
			return testAccNSXPolicyIPBlockCheckDestroy(state)
		},
		Steps: []resource.TestStep{
			{
				Config: testAccNSXPolicyIPBlockCreateMinimalTemplate(name, cidr, false),
				Check: resource.ComposeTestCheckFunc(
					testAccNSXPolicyIPBlockCheckExists(testResourceName),
					resource.TestCheckResourceAttr(testResourceName, "display_name", name),
					resource.TestCheckResourceAttr(testResourceName, "cidr", cidr),
					resource.TestCheckResourceAttr(testResourceName, "tag.#", "0"),
					resource.TestCheckResourceAttrSet(testResourceName, "revision"),
					resource.TestCheckResourceAttrSet(testResourceName, "nsx_id"),
					resource.TestCheckResourceAttrSet(testResourceName, "path"),
				),
			},
		},
	})
}

func TestAccResourceNsxtPolicyIPBlock_basic(t *testing.T) {
	testAccResourceNsxtPolicyIPBlockBasic(t, false, func() {
		testAccPreCheck(t)
		testAccOnlyLocalManager(t)
	})
}

func TestAccResourceNsxtPolicyIPBlock_multitenancy(t *testing.T) {
	testAccResourceNsxtPolicyIPBlockBasic(t, true, func() {
		testAccPreCheck(t)
		testAccOnlyMultitenancy(t)
	})
}

func testAccResourceNsxtPolicyIPBlockBasic(t *testing.T, withContext bool, preCheck func()) {
	name := getAccTestResourceName()
	testResourceName := "nsxt_policy_ip_block.test"
	cidr := "192.168.1.0/24"
	cidr2 := "191.166.1.0/24"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:  preCheck,
		Providers: testAccProviders,
		CheckDestroy: func(state *terraform.State) error {
			return testAccNSXPolicyIPBlockCheckDestroy(state)
		},
		Steps: []resource.TestStep{
			{
				Config: testAccNSXPolicyIPBlockCreateMinimalTemplate(name, cidr, withContext),
				Check: resource.ComposeTestCheckFunc(
					testAccNSXPolicyIPBlockCheckExists(testResourceName),
					resource.TestCheckResourceAttr(testResourceName, "display_name", name),
					resource.TestCheckResourceAttr(testResourceName, "cidr", cidr),
					resource.TestCheckResourceAttr(testResourceName, "tag.#", "0"),
					resource.TestCheckResourceAttrSet(testResourceName, "revision"),
					resource.TestCheckResourceAttrSet(testResourceName, "nsx_id"),
					resource.TestCheckResourceAttrSet(testResourceName, "path"),
				),
			},
			{
				Config: testAccNSXPolicyIPBlockUpdateTemplate(name, cidr2, withContext),
				Check: resource.ComposeTestCheckFunc(
					testAccNSXPolicyIPBlockCheckExists(testResourceName),
					resource.TestCheckResourceAttr(testResourceName, "display_name", name),
					resource.TestCheckResourceAttr(testResourceName, "cidr", cidr2),
					resource.TestCheckResourceAttr(testResourceName, "tag.#", "2"),
					resource.TestCheckResourceAttrSet(testResourceName, "revision"),
					resource.TestCheckResourceAttrSet(testResourceName, "nsx_id"),
					resource.TestCheckResourceAttrSet(testResourceName, "path"),
				),
			},
		},
	})
}

func TestAccResourceNsxtPolicyIPBlock_importBasic(t *testing.T) {
	name := getAccTestResourceName()
	testResourceName := "nsxt_policy_ip_block.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:  func() { testAccOnlyLocalManager(t); testAccPreCheck(t) },
		Providers: testAccProviders,
		CheckDestroy: func(state *terraform.State) error {
			return testAccNSXPolicyIPBlockCheckDestroy(state)
		},
		Steps: []resource.TestStep{
			{
				Config: testAccNSXPolicyIPBlockCreateMinimalTemplate(name, "192.191.1.0/24", false),
			},
			{
				ResourceName:      testResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccResourceNsxtPolicyIPBlock_importBasic_multitenancy(t *testing.T) {
	name := getAccTestResourceName()
	testResourceName := "nsxt_policy_ip_block.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:  func() { testAccPreCheck(t); testAccOnlyMultitenancy(t) },
		Providers: testAccProviders,
		CheckDestroy: func(state *terraform.State) error {
			return testAccNSXPolicyIPBlockCheckDestroy(state)
		},
		Steps: []resource.TestStep{
			{
				Config: testAccNSXPolicyIPBlockCreateMinimalTemplate(name, "192.191.1.0/24", true),
			},
			{
				ResourceName:      testResourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: testAccResourceNsxtPolicyImportIDRetriever(testResourceName),
			},
		},
	})
}

func testAccNSXPolicyIPBlockCheckExists(resourceName string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		connector := getPolicyConnector(testAccProvider.Meta().(nsxtClients))
		client := infra.NewIpBlocksClient(testAccGetSessionContext(), connector)

		rs, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("NSX Policy IP Block resource %s not found in resources", resourceName)
		}

		resourceID := rs.Primary.ID
		if resourceID == "" {
			return fmt.Errorf("NSX Policy IP Block resource ID not set in resources ")
		}

		_, err := client.Get(resourceID)
		if err != nil {
			return fmt.Errorf("Error while retrieving policy IP Block ID %s. Error: %v", resourceID, err)
		}

		return nil
	}
}

func testAccNSXPolicyIPBlockCheckDestroy(state *terraform.State) error {
	connector := getPolicyConnector(testAccProvider.Meta().(nsxtClients))
	client := infra.NewIpBlocksClient(testAccGetSessionContext(), connector)

	for _, rs := range state.RootModule().Resources {
		if rs.Type != "nsxt_policy_ip_block" {
			continue
		}

		resourceID := rs.Primary.Attributes["id"]
		_, err := client.Get(resourceID)
		if err == nil {
			return fmt.Errorf("Policy IP Block %s still exists", resourceID)
		}
	}
	return nil
}

func testAccNSXPolicyIPBlockCreateMinimalTemplate(displayName string, cidr string, withContext bool) string {
	context := ""
	if withContext {
		context = testAccNsxtPolicyMultitenancyContext()
	}
	return fmt.Sprintf(`
resource "nsxt_policy_ip_block" "test" {
%s
  display_name = "%s"
  cidr         = "%s"
}`, context, displayName, cidr)
}

func testAccNSXPolicyIPBlockUpdateTemplate(displayName string, cidr string, withContext bool) string {
	context := ""
	if withContext {
		context = testAccNsxtPolicyMultitenancyContext()
	}
	return fmt.Sprintf(`
resource "nsxt_policy_ip_block" "test" {
%s
  display_name = "%s"
  cidr         = "%s"

  tag {
    scope = "scope1"
    tag   = "tag1"
  }

  tag {
    scope = "scope2"
    tag   = "tag2"
  }
}`, context, displayName, cidr)
}
