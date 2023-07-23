/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: MPL-2.0 */

package nsxt

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

var vpcShortID = getAccTestRandomString(6)

var accTestPolicyVPCCreateAttributes = map[string]string{
	"display_name":    getAccTestResourceName(),
	"description":     "terraform created",
	"short_id":        vpcShortID,
	"ip_address_type": "IPV4",
}

var accTestPolicyVPCUpdateAttributes = map[string]string{
	"display_name":    getAccTestResourceName(),
	"description":     "terraform updated",
	"short_id":        vpcShortID,
	"ip_address_type": "IPV4",
}

func TestAccResourceNsxtPolicyVPC_basic(t *testing.T) {
	testResourceName := "nsxt_policy_vpc.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccOnlyLocalManager(t)
			testAccNSXVersion(t, "4.1.0")
		},
		Providers: testAccProviders,
		CheckDestroy: func(state *terraform.State) error {
			return testAccNsxtPolicyVPCCheckDestroy(state, accTestPolicyVPCUpdateAttributes["display_name"])
		},
		Steps: []resource.TestStep{
			{
				Config: testAccNsxtPolicyVPCTemplate(true),
				Check: resource.ComposeTestCheckFunc(
					testAccNsxtPolicyVPCExists(accTestPolicyVPCCreateAttributes["display_name"], testResourceName),
					resource.TestCheckResourceAttr(testResourceName, "display_name", accTestPolicyVPCCreateAttributes["display_name"]),
					resource.TestCheckResourceAttr(testResourceName, "description", accTestPolicyVPCCreateAttributes["description"]),
					resource.TestCheckResourceAttr(testResourceName, "short_id", accTestPolicyVPCCreateAttributes["short_id"]),
					resource.TestCheckResourceAttr(testResourceName, "ip_address_type", accTestPolicyVPCCreateAttributes["ip_address_type"]),
					//TODO: add site_info, load_balancer_vpc_endpoint, default_gateway_path, service_gateway, private_ipv4_blocks, external_ipv4_blocks, ipv6_profile_paths, dhcp_config property validation
					resource.TestCheckResourceAttr(testResourceName, "site_info.#", "0"),
					resource.TestCheckResourceAttr(testResourceName, "external_ipv4_blocks.#", "1"),

					resource.TestCheckResourceAttrSet(testResourceName, "nsx_id"),
					resource.TestCheckResourceAttrSet(testResourceName, "path"),
					resource.TestCheckResourceAttrSet(testResourceName, "revision"),
					resource.TestCheckResourceAttr(testResourceName, "tag.#", "1"),
				),
			},
			{
				Config: testAccNsxtPolicyVPCTemplate(false),
				Check: resource.ComposeTestCheckFunc(
					testAccNsxtPolicyVPCExists(accTestPolicyVPCUpdateAttributes["display_name"], testResourceName),
					resource.TestCheckResourceAttr(testResourceName, "display_name", accTestPolicyVPCUpdateAttributes["display_name"]),
					resource.TestCheckResourceAttr(testResourceName, "description", accTestPolicyVPCUpdateAttributes["description"]),
					resource.TestCheckResourceAttr(testResourceName, "short_id", accTestPolicyVPCCreateAttributes["short_id"]),
					resource.TestCheckResourceAttr(testResourceName, "ip_address_type", accTestPolicyVPCCreateAttributes["ip_address_type"]),
					resource.TestCheckResourceAttr(testResourceName, "site_info.#", "0"),
					resource.TestCheckResourceAttr(testResourceName, "external_ipv4_blocks.#", "1"),

					resource.TestCheckResourceAttrSet(testResourceName, "nsx_id"),
					resource.TestCheckResourceAttrSet(testResourceName, "path"),
					resource.TestCheckResourceAttrSet(testResourceName, "revision"),
					resource.TestCheckResourceAttr(testResourceName, "tag.#", "1"),
				),
			},
			{
				Config: testAccNsxtPolicyVPCTemplate(false),
				Check: resource.ComposeTestCheckFunc(
					testAccNsxtPolicyVPCExists(accTestPolicyVPCUpdateAttributes["display_name"], testResourceName),
					resource.TestCheckResourceAttr(testResourceName, "display_name", accTestPolicyVPCUpdateAttributes["display_name"]),
					resource.TestCheckResourceAttr(testResourceName, "description", accTestPolicyVPCUpdateAttributes["description"]),
					resource.TestCheckResourceAttr(testResourceName, "short_id", accTestPolicyVPCCreateAttributes["short_id"]),
					resource.TestCheckResourceAttr(testResourceName, "ip_address_type", accTestPolicyVPCCreateAttributes["ip_address_type"]),
					resource.TestCheckResourceAttr(testResourceName, "site_info.#", "0"),
					resource.TestCheckResourceAttr(testResourceName, "external_ipv4_blocks.#", "0"),

					resource.TestCheckResourceAttrSet(testResourceName, "nsx_id"),
					resource.TestCheckResourceAttrSet(testResourceName, "path"),
					resource.TestCheckResourceAttrSet(testResourceName, "revision"),
					resource.TestCheckResourceAttr(testResourceName, "tag.#", "1"),
				),
			},
			{
				Config: testAccNsxtPolicyVPCMinimalistic(),
				Check: resource.ComposeTestCheckFunc(
					testAccNsxtPolicyVPCExists(accTestPolicyVPCCreateAttributes["display_name"], testResourceName),
					resource.TestCheckResourceAttr(testResourceName, "description", ""),
					resource.TestCheckResourceAttrSet(testResourceName, "nsx_id"),
					resource.TestCheckResourceAttrSet(testResourceName, "path"),
					resource.TestCheckResourceAttrSet(testResourceName, "revision"),
					resource.TestCheckResourceAttr(testResourceName, "tag.#", "0"),
				),
			},
		},
	})
}

func TestAccResourceNsxtPolicyVPC_importBasic(t *testing.T) {
	name := getAccTestResourceName()
	testResourceName := "nsxt_policy_vpc.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccOnlyLocalManager(t)
			testAccNSXVersion(t, "4.1.0")
		},
		Providers: testAccProviders,
		CheckDestroy: func(state *terraform.State) error {
			return testAccNsxtPolicyVPCCheckDestroy(state, name)
		},
		Steps: []resource.TestStep{
			{
				Config: testAccNsxtPolicyVPCMinimalistic(),
			},
			{
				ResourceName:      testResourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccNsxtPolicyVPCExists(displayName string, resourceName string) resource.TestCheckFunc {
	return func(state *terraform.State) error {

		connector := getPolicyConnector(testAccProvider.Meta().(nsxtClients))

		rs, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("Policy VPC resource %s not found in resources", resourceName)
		}

		resourceID := rs.Primary.ID
		if resourceID == "" {
			return fmt.Errorf("Policy VPC resource ID not set in resources")
		}

		exists, err := resourceNsxtPolicyVPCExists(resourceID, connector, testAccIsGlobalManager())
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("Policy VPC %s does not exist", resourceID)
		}

		return nil
	}
}

func testAccNsxtPolicyVPCCheckDestroy(state *terraform.State, displayName string) error {
	connector := getPolicyConnector(testAccProvider.Meta().(nsxtClients))
	for _, rs := range state.RootModule().Resources {

		if rs.Type != "nsxt_policy_vpc" {
			continue
		}

		resourceID := rs.Primary.Attributes["id"]
		exists, err := resourceNsxtPolicyVPCExists(resourceID, connector, testAccIsGlobalManager())
		if err == nil {
			return err
		}

		if exists {
			return fmt.Errorf("Policy VPC %s still exists", displayName)
		}
	}
	return nil
}

func testAccNsxtPolicyVPCTemplate(isCreateRequest bool) string {
	var attrMap map[string]string
	shortIDSpec := ""
	if isCreateRequest {
		attrMap = accTestPolicyVPCCreateAttributes
		shortIDSpec = fmt.Sprintf("short_id     = \"%s\"", attrMap["short_id"])
	} else {
		attrMap = accTestPolicyVPCUpdateAttributes
	}
	default_gateway_path := "default_gateway_path = data.nsxt_policy_tier0_gateway.test.path"
	external_ipv4_block := "external_ipv4_blocks = [data.nsxt_policy_ip_block.test.path]"
	return fmt.Sprintf(`
	 data "nsxt_policy_tier0_gateway" "test" {
		 display_name = "%s"
	 }

	 data "nsxt_policy_ip_block" "test" {
		context {
			project_id = "%s"
		}
		display_name = "%s"
	}

	 resource "nsxt_policy_vpc" "test" {
		 project_id   = "%s"
		 display_name = "%s"
		 description  = "%s"
		 %s
		 %s
		 %s
		 %s
		 %s
		 %s
	 
		 tag {
			 scope = "scope1"
			 tag   = "tag1"
		 }
	 }`, getTier0RouterName(), getProjectName(), getIpBlockName(), getProjectName(), attrMap["display_name"], attrMap["description"], shortIDSpec, default_gateway_path, attrMap["ip_address_type"], external_ipv4_block)
}

func testAccNsxtPolicyVPCMinimalistic() string {
	return fmt.Sprintf(`
	 resource "nsxt_policy_vpc" "test" {
		 project_id   = "%s"
		 display_name = "%s"
	 
	 }`, getProjectName(), accTestPolicyVPCUpdateAttributes["display_name"])
}
