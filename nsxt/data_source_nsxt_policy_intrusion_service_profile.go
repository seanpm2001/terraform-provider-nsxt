/* Copyright © 2021 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: MPL-2.0 */

package nsxt

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceNsxtPolicyIntrusionServiceProfile() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceNsxtPolicyIntrusionServiceProfileRead,

		Schema: map[string]*schema.Schema{
			"id":           getDataSourceIDSchema(),
			"display_name": getDataSourceExtendedDisplayNameSchema(),
			"description":  getDataSourceDescriptionSchema(),
			"path":         getPathSchema(),
		},
	}
}

func dataSourceNsxtPolicyIntrusionServiceProfileRead(d *schema.ResourceData, m interface{}) error {
	connector := getPolicyConnector(m)

	if isPolicyGlobalManager(m) {
		return localManagerOnlyError()
	}

	_, err := policyDataSourceResourceRead(d, connector, getSessionContext(d, m), "IdsProfile", nil)
	if err != nil {
		return err
	}

	return nil
}
