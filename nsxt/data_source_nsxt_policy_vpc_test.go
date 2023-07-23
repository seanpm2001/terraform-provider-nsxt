/* Copyright © 2023 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: MPL-2.0 */

package nsxt

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt/model"
	projects "github.com/vmware/vsphere-automation-sdk-go/services/nsxt/orgs/projects"
)

func TestAccDataSourceNsxtPolicyVPC_basic(t *testing.T) {
	name := getAccTestDataSourceName()
	testResourceName := "data.nsxt_policy_vpc.test"
	projectId := getProjectName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck: func() {
			testAccOnlyLocalManager(t)
			testAccPreCheck(t)
			testAccNSXVersion(t, "4.1.0")
		},
		Providers: testAccProviders,
		CheckDestroy: func(state *terraform.State) error {
			return testAccDataSourceNsxtPolicyVPCDeleteByName(name, projectId)
		},
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					if err := testAccDataSourceNsxtPolicyVPCCreate(name, projectId); err != nil {
						panic(err)
					}
				},
				Config: testAccNsxtPolicyVPCReadTemplate(name),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(testResourceName, "display_name", name),
					resource.TestCheckResourceAttr(testResourceName, "description", name),
					resource.TestCheckResourceAttrSet(testResourceName, "path"),
					resource.TestCheckResourceAttr(testResourceName, "tier0_gateway_paths.#", "1"),
					// TODO: add other properties checks
				),
			},
		},
	})
}

func testAccDataSourceNsxtPolicyVPCCreate(name string, projectId string) error {
	connector, err := testAccGetPolicyConnector()
	if err != nil {
		return fmt.Errorf("Error during test client initialization: %v", err)
	}

	var defaulGatewayPath = getTier0RouterPath(connector)
	var ipAddressType = "IPV4"
	client := projects.NewVpcsClient(connector)

	displayName := name
	description := name
	obj := model.Vpc{
		Description:        &description,
		DisplayName:        &displayName,
		DefaultGatewayPath: &defaulGatewayPath,
		IpAddressType:      &ipAddressType,
	}

	// Generate a random ID for the resource
	id := newUUID()

	err = client.Patch(defaultOrgID, projectId, id, obj)
	if err != nil {
		return handleCreateError("Vpc", id, err)
	}
	return nil
}

func testAccDataSourceNsxtPolicyVPCDeleteByName(name string, projectId string) error {
	connector, err := testAccGetPolicyConnector()
	if err != nil {
		return fmt.Errorf("Error during test client initialization: %v", err)
	}
	client := projects.NewVpcsClient(connector)

	// Find the object by name
	objList, err := client.List(defaultOrgID, projectId, nil, nil, nil, nil, nil, nil)
	if err != nil {
		return handleListError("Vpc", err)
	}
	for _, objInList := range objList.Results {
		if *objInList.DisplayName == name {
			err := client.Delete(defaultOrgID, projectId, *objInList.Id)
			if err != nil {
				return handleDeleteError("Vpc", *objInList.Id, err)
			}
			return nil
		}
	}
	return fmt.Errorf("Error while deleting VPC '%s': resource not found", name)
}

func testAccNsxtPolicyVPCReadTemplate(name string) string {
	return fmt.Sprintf(`
	 data "nsxt_policy_vpc" "test" {
		 project_id = "%s"
		 display_name = "%s"
	 }`, getProjectName(), name)
}
