/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: MPL-2.0 */

package nsxt

import (
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/vmware/vsphere-automation-sdk-go/runtime/protocol/client"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt/model"
	projects "github.com/vmware/vsphere-automation-sdk-go/services/nsxt/orgs/projects"
)

var projectID = ""

func resourceNsxtPolicyVPC() *schema.Resource {
	return &schema.Resource{
		Create: resourceNsxtPolicyVPCCreate,
		Read:   resourceNsxtPolicyVPCRead,
		Update: resourceNsxtPolicyVPCUpdate,
		Delete: resourceNsxtPolicyVPCDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"project_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"nsx_id":       getNsxIDSchema(),
			"path":         getPathSchema(),
			"display_name": getDisplayNameSchema(),
			"description":  getDescriptionSchema(),
			"revision":     getRevisionSchema(),
			"tag":          getTagsSchema(),
			"short_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
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

func getPolicySubnetProfileSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Description: "Subnet profile",
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"ip_discovery": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "IP Discovery Profile",
				},
				"mac_discovery": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Mac Discovery Profile",
				},
				"qos": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Segment qos Profile",
				},
				"segment_security": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Segment security Profile",
				},
				"spoof_guard": {
					Type:        schema.TypeString,
					Optional:    true,
					Description: "Spoofguard Profile",
				},
			},
		},
	}
}

func getPolicyDHCPConfigSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Description: "DHCP configuration to be applied on all connected subnets if the IP address type is IPv4",
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"dhcp_relay_config_path": getPolicyPathSchema(false, false, "Policy path to DHCP server or relay configuration to use for this Tier0"),
				"enable_dhcp": {
					Type:        schema.TypeBool,
					Optional:    true,
					Description: "Enable or Disable DHCP",
				},
				"dns_client_config": {
					Type: schema.TypeList,
					Elem: &schema.Resource{
						Schema: map[string]*schema.Schema{
							"dns_server_ips": {
								Type:        schema.TypeList,
								Elem:        &schema.Schema{Type: schema.TypeString},
								Optional:    true,
								Description: "IPs of the DNS servers which need to be configured on the workload VMs",
							},
						},
					},
					Optional:    true,
					MaxItems:    1,
					Description: "DNS client configuration",
				},
			},
		},
	}
}

func getPolicyServiceGatewaySchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Description: "Service gateway configuration",
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"auto_snat": {
					Type:     schema.TypeBool,
					Optional: true,
					Default:  true,
				},
				"disable": {
					Type:     schema.TypeBool,
					Optional: true,
					Default:  false,
				},
				"qos_config": getQOSConfigSchema(),
			},
		},
	}
}

func getQOSConfigSchema() *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Description: "QOS configuration",
		Optional:    true,
		MaxItems:    1,
		Elem: &schema.Resource{
			Schema: map[string]*schema.Schema{
				"ingress_qos_profile_path": getPolicyPathSchema(false, false, "Policy path to gateway QoS profile in ingress direction"),
				"egress_qos_profile_path":  getPolicyPathSchema(false, false, "Policy path to gateway QoS profile in egress direction"),
			},
		},
	}
}

func resourceNsxtPolicyVPCExists(id string, connector client.Connector, isGlobalManager bool) (bool, error) {
	var err error
	// use vpc client from vmware vendor and get org and project ids
	client := projects.NewVpcsClient(connector)
	_, err = client.Get(defaultOrgID, projectID, id)

	if err == nil {
		return true, nil
	}

	if isNotFoundError(err) {
		return false, nil
	}

	return false, logAPIError("Error retrieving resource", err)
}

func resourceNsxtPolicyVPCPatch(connector client.Connector, d *schema.ResourceData, m interface{}, id string) error {
	projectId := d.Get("project_id").(string)
	displayName := d.Get("display_name").(string)
	description := d.Get("description").(string)
	tags := getPolicyTagsFromSchema(d)
	shortID := d.Get("short_id").(string)
	defaultGatewayPath := d.Get("default_gateway_path").(string)
	siteInfosList := d.Get("site_info").([]interface{})
	subnetProfile := d.Get("subnet_profile").(interface{})
	loadBalancerVpcEndpoint := d.Get("load_balancer_vpc_endpoint").(interface{})
	serviceGateway := d.Get("service_gateway").(interface{})
	ipAddressType := d.Get("ip_address_type").(string)
	dhcpConfig := d.Get("dhcp_config").(interface{})

	// process site_info
	var siteInfos []model.SiteInfo
	for _, item := range siteInfosList {
		data := item.(map[string]interface{})
		var edgeClusterPaths []string
		for _, ecp := range data["edge_cluster_paths"].([]interface{}) {
			edgeClusterPaths = append(edgeClusterPaths, ecp.(string))
		}
		sitePath := data["site_path"].(string)
		siteInfoObj := model.SiteInfo{
			EdgeClusterPaths: edgeClusterPaths,
			SitePath:         &sitePath,
		}
		siteInfos = append(siteInfos, siteInfoObj)
	}

	// process subnet_profile
	subnetProfileData := subnetProfile.(map[string]interface{})
	ipDiscovery := subnetProfileData["ip_discovery"].(string)
	macDiscovery := subnetProfileData["mac_discovery"].(string)
	qos := subnetProfileData["qos"].(string)
	segmentSecurity := subnetProfileData["segment_security"].(string)
	spoofGuard := subnetProfileData["spoof_guard"].(string)
	subnetProfileObj := model.SubnetProfiles{
		IpDiscovery:     &ipDiscovery,
		MacDiscovery:    &macDiscovery,
		Qos:             &qos,
		SegmentSecurity: &segmentSecurity,
		SpoofGuard:      &spoofGuard,
	}

	// process load_balancer_vpc_endpoint
	loadBalancerVpcEndpointData := loadBalancerVpcEndpoint.(map[string]interface{})
	enabled := loadBalancerVpcEndpointData["enabled"].(bool)
	loadBalancerVpcEndpointObj := model.LoadBalancerVPCEndpoint{
		Enabled: &enabled,
	}

	// process service gateway
	serviceGatewayData := serviceGateway.(map[string]interface{})
	autoSnat := serviceGatewayData["auto_snat"].(bool)
	disable := serviceGatewayData["disable"].(bool)
	qosConfig := serviceGatewayData["qos_config"].(interface{})

	qosConfigData := qosConfig.(map[string]interface{})
	ingressQosProfilePath := qosConfigData["ingress_qos_profile_path"].(string)
	egressQosProfilePath := qosConfigData["egress_qos_profile_path"].(string)
	qosObj := model.GatewayQosProfileConfig{
		IngressQosProfilePath: &ingressQosProfilePath,
		EgressQosProfilePath:  &egressQosProfilePath,
	}

	serviceGatewayObj := model.ServiceGateway{
		AutoSnat:  &autoSnat,
		Disable:   &disable,
		QosConfig: &qosObj,
	}

	// process private_ipv4_blocks, external_ipv4_blocks, ipv6_profile_paths
	privateIpv4Blocks := getStringListFromSchemaList(d, "private_ipv4_blocks")
	externalIpv4Blocks := getStringListFromSchemaList(d, "external_ipv4_blocks")
	ipv6ProfilePaths := getStringListFromSchemaList(d, "ipv6_profile_paths")

	// process dhcp config
	dhcpConfigData := dhcpConfig.(map[string]interface{})
	dhcpRelayConfigPath := dhcpConfigData["dhcp_relay_config_path"].(string)
	enableDhcp := dhcpConfigData["enable_dhcp"].(bool)
	dnsClientConfig := dhcpConfigData["dns_client_config"].(interface{})

	dnsClientConfigData := dnsClientConfig.(map[string]interface{})
	dnsServerIps := dnsClientConfigData["dns_server_ips"].([]string)
	dnsClientConfigObj := model.DnsClientConfig{
		DnsServerIps: dnsServerIps,
	}

	dhcpConfigObj := model.DhcpConfig{
		DhcpRelayConfigPath: &dhcpRelayConfigPath,
		EnableDhcp:          &enableDhcp,
		DnsClientConfig:     &dnsClientConfigObj,
	}

	vpcObj := model.Vpc{
		DisplayName:             &displayName,
		Description:             &description,
		Tags:                    tags,
		SiteInfos:               siteInfos,
		DefaultGatewayPath:      &defaultGatewayPath,
		ServiceGateway:          &serviceGatewayObj,
		SubnetProfiles:          &subnetProfileObj,
		IpAddressType:           &ipAddressType,
		PrivateIpv4Blocks:       privateIpv4Blocks,
		ExternalIpv4Blocks:      externalIpv4Blocks,
		Ipv6ProfilePaths:        ipv6ProfilePaths,
		DhcpConfig:              &dhcpConfigObj,
		LoadBalancerVpcEndpoint: &loadBalancerVpcEndpointObj,
	}

	if shortID != "" {
		vpcObj.ShortId = &shortID
	}

	log.Printf("[INFO] Patching Vpc with ID %s", id)

	client := projects.NewVpcsClient(connector)
	return client.Patch(defaultOrgID, projectId, id, vpcObj)
}

func resourceNsxtPolicyVPCCreate(d *schema.ResourceData, m interface{}) error {
	projectID = d.Get("project_id").(string)
	// Initialize resource Id and verify this ID is not yet used
	id, err := getOrGenerateID(d, m, resourceNsxtPolicyVPCExists)
	if err != nil {
		return err
	}

	connector := getPolicyConnector(m)
	err = resourceNsxtPolicyVPCPatch(connector, d, m, id)
	if err != nil {
		return handleCreateError("Vpc", id, err)
	}

	d.SetId(id)
	d.Set("nsx_id", id)

	return resourceNsxtPolicyVPCRead(d, m)
}

func resourceNsxtPolicyVPCRead(d *schema.ResourceData, m interface{}) error {
	connector := getPolicyConnector(m)
	projectId := d.Get("project_id").(string)
	id := d.Id()
	if id == "" {
		return fmt.Errorf("Error obtaining VPC ID")
	}

	var obj model.Vpc
	client := projects.NewVpcsClient(connector)
	var err error
	obj, err = client.Get(defaultOrgID, projectId, id)
	if err != nil {
		return handleReadError(d, "Vpc", id, err)
	}

	d.Set("display_name", obj.DisplayName)
	d.Set("description", obj.Description)
	setPolicyTagsInSchema(d, obj.Tags)
	d.Set("nsx_id", id)
	d.Set("path", obj.Path)
	d.Set("revision", obj.Revision)

	d.Set("short_id", obj.ShortId)

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

func resourceNsxtPolicyVPCUpdate(d *schema.ResourceData, m interface{}) error {

	id := d.Id()
	if id == "" {
		return fmt.Errorf("Error obtaining VPC ID")
	}

	connector := getPolicyConnector(m)
	err := resourceNsxtPolicyVPCPatch(connector, d, m, id)
	if err != nil {
		return handleUpdateError("Vpc", id, err)
	}

	return resourceNsxtPolicyVPCRead(d, m)
}

func resourceNsxtPolicyVPCDelete(d *schema.ResourceData, m interface{}) error {
	projectId := d.Get("project_id").(string)
	id := d.Id()
	if id == "" {
		return fmt.Errorf("Error obtaining VPC ID")
	}

	connector := getPolicyConnector(m)
	var err error
	client := projects.NewVpcsClient(connector)
	err = client.Delete(defaultOrgID, projectId, id)

	if err != nil {
		return handleDeleteError("Vpc", id, err)
	}

	return nil
}
