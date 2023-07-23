/* Copyright © 2023 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: MPL-2.0 */

package nsxt

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt/model"
	projects "github.com/vmware/vsphere-automation-sdk-go/services/nsxt/orgs/projects"
)

func dataSourceNsxtPolicyVPC() *schema.Resource {
	return &schema.Resource{
		Read: dataSourceNsxtPolicyVPCRead,

		Schema: map[string]*schema.Schema{
			"project_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"id":           getDataSourceIDSchema(),
			"display_name": getDataSourceDisplayNameSchema(),
			"description":  getDataSourceDescriptionSchema(),
			"path":         getPathSchema(),
			"short_id": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"site_info": {
				Type: schema.TypeList,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"edge_cluster_paths": {
							Type:     schema.TypeList,
							Elem:     getElemPolicyPathSchemaWithFlags(false, false, false),
							Optional: true,
						},
						"site_path": getElemPolicyPathSchemaWithFlags(true, false, false),
					},
				},
				Optional:    true,
				Description: "Information related to sites applicable for given VPC",
			},
			"subnet_profile": getPolicySubnetProfileSchema(),
			"load_balancer_vpc_endpoint": {
				Type: schema.TypeList,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"enabled": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  false,
						},
					},
				},
				Optional: true,
			},
			"default_gateway_path": {
				Type:        schema.TypeString,
				Elem:        getElemPolicyPathSchema(),
				Optional:    true,
				Description: "PolicyPath of Tier0 or Tier0 VRF gateway or label path referencing to Tier0 or Tier0 VRF",
			},
			"service_gateway": getPolicyServiceGatewaySchema(),
			"ip_address_type": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringInSlice([]string{"IPV4"}, false),
				Default:      "IPV4",
				Description:  "This defines the IP address type that will be allocated for subnets",
			},
			"private_ipv4_blocks": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    5,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "PolicyPath of private ip block",
			},
			"external_ipv4_blocks": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    5,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "PolicyPath of external IPv4 block",
			},
			"ipv6_profile_paths": {
				Type:        schema.TypeList,
				Optional:    true,
				MaxItems:    2,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Description: "IPv6 NDRA and DAD profiles configuration",
			},
			"dhcp_config": getPolicyDHCPConfigSchema(),
		},
	}
}

func dataSourceNsxtPolicyVPCRead(d *schema.ResourceData, m interface{}) error {
	connector := getPolicyConnector(m)
	client := projects.NewVpcsClient(connector)

	// As VPC resource type paths reside under project and not under /infra or /global_infra or such, and since
	// this data source fetches extra attributes, e.g site_info and tier0_gateway_paths, it's simpler to implement using .List()
	// instead of using search API.

	objID := d.Get("id").(string)
	objName := d.Get("display_name").(string)
	projectId := d.Get("project_id").(string)
	var obj model.Vpc
	if objID != "" {
		// Get by id
		objGet, err := client.Get(defaultOrgID, projectId, objID)
		if err != nil {
			return handleDataSourceReadError(d, "Vpc", objID, err)
		}
		obj = objGet
	} else if objName == "" {
		return fmt.Errorf("Error obtaining Vpc ID or name during read")
	} else {
		// Get by full name/prefix
		objList, err := client.List(defaultOrgID, projectId, nil, nil, nil, nil, nil, nil)
		if err != nil {
			return handleListError("Vpc", err)
		}
		// go over the list to find the correct one (prefer a perfect match. If not - prefix match)
		var perfectMatch []model.Vpc
		var prefixMatch []model.Vpc
		for _, objInList := range objList.Results {
			if strings.HasPrefix(*objInList.DisplayName, objName) {
				prefixMatch = append(prefixMatch, objInList)
			}
			if *objInList.DisplayName == objName {
				perfectMatch = append(perfectMatch, objInList)
			}
		}
		if len(perfectMatch) > 0 {
			if len(perfectMatch) > 1 {
				return fmt.Errorf("Found multiple VPCs with name '%s'", objName)
			}
			obj = perfectMatch[0]
		} else if len(prefixMatch) > 0 {
			if len(prefixMatch) > 1 {
				return fmt.Errorf("Found multiple VPCs with name starting with '%s'", objName)
			}
			obj = prefixMatch[0]
		} else {
			return fmt.Errorf("VPC with name '%s' was not found", objName)
		}
	}

	d.SetId(*obj.Id)
	d.Set("display_name", obj.DisplayName)
	d.Set("description", obj.Description)
	d.Set("path", obj.Path)

	// process site_info
	var siteInfosList []map[string]interface{}
	for _, item := range obj.SiteInfos {
		data := make(map[string]interface{})
		data["edge_cluster_paths"] = item.EdgeClusterPaths
		data["site_path"] = item.SitePath
		siteInfosList = append(siteInfosList, data)
	}
	d.Set("site_info", siteInfosList)

	// process subnet_profile
	subnetProfileData := make(map[string]interface{})
	subnetProfileData["ip_discovery"] = obj.SubnetProfiles.IpDiscovery
	subnetProfileData["mac_discovery"] = obj.SubnetProfiles.MacDiscovery
	subnetProfileData["qos"] = obj.SubnetProfiles.Qos
	subnetProfileData["segment_security"] = obj.SubnetProfiles.SegmentSecurity
	subnetProfileData["spoof_guard"] = obj.SubnetProfiles.SpoofGuard

	d.Set("subnet_profile", subnetProfileData)

	// process load_balancer_vpc_endpoint
	loadBalancerVpcEndpointData := make(map[string]interface{})
	loadBalancerVpcEndpointData["enabled"] = obj.LoadBalancerVpcEndpoint.Enabled
	d.Set("load_balancer_vpc_endpoint", loadBalancerVpcEndpointData)

	// process default_gateway_path
	d.Set("default_gateway_path", obj.DefaultGatewayPath)

	// process ip_address_type
	d.Set("ip_address_type", obj.IpAddressType)

	// process service_gateway
	serviceGatewayData := make(map[string]interface{})
	serviceGatewayData["auto_snat"] = obj.ServiceGateway.AutoSnat
	serviceGatewayData["disable"] = obj.ServiceGateway.Disable
	qosConfigData := make(map[string]interface{})
	qosConfigData["egress_qos_profile_path"] = obj.ServiceGateway.QosConfig.EgressQosProfilePath
	qosConfigData["ingress_qos_profile_path"] = obj.ServiceGateway.QosConfig.IngressQosProfilePath
	serviceGatewayData["qos_config"] = qosConfigData
	d.Set("service_gateway", serviceGatewayData)

	// process private_ipv4_blocks, external_ipv4_blocks, ipv6_profile_paths
	d.Set("private_ipv4_blocks", obj.PrivateIpv4Blocks)
	d.Set("external_ipv4_blocks", obj.ExternalIpv4Blocks)
	d.Set("ipv6_profile_paths", obj.Ipv6ProfilePaths)

	// process dhcp_config
	dhcpConfigData := make(map[string]interface{})
	dhcpConfigData["enable_dhcp"] = obj.DhcpConfig.EnableDhcp
	dhcpConfigData["dhcp_relay_config_path"] = obj.DhcpConfig.DhcpRelayConfigPath
	dnsClientConfigData := make(map[string]interface{})
	dnsClientConfigData["dns_server_ips"] = obj.DhcpConfig.DnsClientConfig.DnsServerIps
	dhcpConfigData["dns_client_config"] = dnsClientConfigData
	d.Set("dhcp_config", dhcpConfigData)

	return nil
}
