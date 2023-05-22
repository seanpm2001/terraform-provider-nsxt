/* Copyright © 2019 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: MPL-2.0 */

package nsxt

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func dataSourceNsxtPolicyDhcpServer() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceNsxtPolicyDhcpServerRead,

		Schema: map[string]*schema.Schema{
			"id":           getDataSourceIDSchema(),
			"display_name": getDataSourceExtendedDisplayNameSchema(),
			"description":  getDataSourceDescriptionSchema(),
			"path":         getPathSchema(),
			"context":      getContextSchema(),
		},
	}
}

func dataSourceNsxtPolicyDhcpServerRead(d *schema.ResourceData, m interface{}) error {
	connector := getPolicyConnector(m)

	_, err := policyDataSourceResourceRead(d, connector, getSessionContext(d, m), "DhcpServerConfig", nil)
	if err != nil {
		return err
	}

	return nil
}
