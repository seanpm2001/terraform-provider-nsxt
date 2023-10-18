/* Copyright © 2017 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: MPL-2.0 */

package nsxt

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	api "github.com/vmware/go-vmware-nsxt"
	"github.com/vmware/vsphere-automation-sdk-go/runtime/core"
	"github.com/vmware/vsphere-automation-sdk-go/runtime/protocol/client"
	"github.com/vmware/vsphere-automation-sdk-go/runtime/protocol/client/middleware/retry"
	"github.com/vmware/vsphere-automation-sdk-go/runtime/security"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt-mp/nsx"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt-mp/nsx/model"
	"golang.org/x/exp/slices"

	tf_api "github.com/vmware/terraform-provider-nsxt/api/utl"
)

var defaultRetryOnStatusCodes = []int{400, 409, 429, 500, 503, 504}

// Struct to keep track of changes in schema List
type listDiff struct {
	added   []interface{}
	removed []interface{}
}

// Provider configuration that is shared for policy and MP
type commonProviderConfig struct {
	RemoteAuth             bool
	BearerToken            string
	ToleratePartialSuccess bool
	MaxRetries             int
	MinRetryInterval       int
	MaxRetryInterval       int
	RetryStatusCodes       []int
	Username               string
	Password               string
	LicenseDiff            listDiff
}

type nsxtClients struct {
	CommonConfig commonProviderConfig
	// NSX Manager client - based on go-vmware-nsxt SDK
	NsxtClient *api.APIClient
	// Config for the above client
	NsxtClientConfig *api.Configuration
	// Data for NSX Policy client - based on vsphere-automation-sdk-go SDK
	// First offering of Policy SDK does not support concurrent
	// operations in single connector. In order to avoid heavy locks,
	// we are allocating connector per provider operation.
	// TODO: when concurrency support is introduced policy client,
	// change this code to allocate single connector for all provider
	// operations.
	PolicySecurityContext  *core.SecurityContextImpl
	PolicyHTTPClient       *http.Client
	Host                   string
	PolicyEnforcementPoint string
	PolicyGlobalManager    bool
}

// Provider for VMWare NSX-T
func Provider() *schema.Provider {
	return &schema.Provider{

		Schema: map[string]*schema.Schema{
			"allow_unverified_ssl": {
				Type:        schema.TypeBool,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSXT_ALLOW_UNVERIFIED_SSL", false),
			},
			"username": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSXT_USERNAME", nil),
			},
			"password": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSXT_PASSWORD", nil),
				Sensitive:   true,
			},
			"remote_auth": {
				Type:        schema.TypeBool,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSXT_REMOTE_AUTH", false),
			},
			"session_auth": {
				Type:        schema.TypeBool,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSXT_SESSION_AUTH", true),
			},
			"host": {
				Type:         schema.TypeString,
				Optional:     true,
				DefaultFunc:  schema.EnvDefaultFunc("NSXT_MANAGER_HOST", nil),
				ValidateFunc: validateNsxtProviderHostFormat(),
				Description:  "The hostname or IP address of the NSX manager.",
			},
			"client_auth_cert_file": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSXT_CLIENT_AUTH_CERT_FILE", nil),
			},
			"client_auth_key_file": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSXT_CLIENT_AUTH_KEY_FILE", nil),
			},
			"ca_file": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSXT_CA_FILE", nil),
			},
			"max_retries": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Maximum number of HTTP client retries",
				DefaultFunc: schema.EnvDefaultFunc("NSXT_MAX_RETRIES", 4),
			},
			"retry_min_delay": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Minimum delay in milliseconds between retries of a request",
				DefaultFunc: schema.EnvDefaultFunc("NSXT_RETRY_MIN_DELAY", 0),
			},
			"retry_max_delay": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "Maximum delay in milliseconds between retries of a request",
				DefaultFunc: schema.EnvDefaultFunc("NSXT_RETRY_MAX_DELAY", 500),
			},
			"retry_on_status_codes": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "HTTP replies status codes to retry on",
				Elem: &schema.Schema{
					Type: schema.TypeInt,
				},
				// There is no support for default values/func for list, so it will be handled later
			},
			"tolerate_partial_success": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Treat partial success status as success",
				DefaultFunc: schema.EnvDefaultFunc("NSXT_TOLERATE_PARTIAL_SUCCESS", false),
			},
			"vmc_auth_host": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "URL for VMC authorization service (CSP)",
				DefaultFunc: schema.EnvDefaultFunc("NSXT_VMC_AUTH_HOST", "console.cloud.vmware.com/csp/gateway/am/api/auth/api-tokens/authorize"),
			},
			"vmc_token": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Long-living API token for VMC authorization",
				DefaultFunc: schema.EnvDefaultFunc("NSXT_VMC_TOKEN", nil),
			},
			"vmc_auth_mode": {
				Type:         schema.TypeString,
				Optional:     true,
				DefaultFunc:  schema.EnvDefaultFunc("NSXT_VMC_AUTH_MODE", "Default"),
				ValidateFunc: validation.StringInSlice([]string{"Default", "Bearer", "Basic"}, false),
				Description:  "Mode for VMC authorization",
			},
			"enforcement_point": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Enforcement Point for NSXT Policy",
				DefaultFunc: schema.EnvDefaultFunc("NSXT_POLICY_ENFORCEMENT_POINT", "default"),
			},
			"global_manager": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Is this a policy global manager endpoint",
				DefaultFunc: schema.EnvDefaultFunc("NSXT_GLOBAL_MANAGER", false),
			},
			"license_keys": {
				Type:          schema.TypeList,
				Optional:      true,
				Description:   "license keys",
				ConflictsWith: []string{"vmc_token"},
				Elem: &schema.Schema{
					Type: schema.TypeString,
					ValidateFunc: validation.StringMatch(
						regexp.MustCompile(
							"^[A-Z0-9]{5}-[A-Z0-9]{5}-[A-Z0-9]{5}-[A-Z0-9]{5}-[A-Z0-9]{5}$"),
						"Must be a valid nsx license key matching: ^[A-Z0-9]{5}-[A-Z0-9]{5}-[A-Z0-9]{5}-[A-Z0-9]{5}-[A-Z0-9]{5}$"),
				},
			},
			"client_auth_cert": {
				Type:        schema.TypeString,
				Description: "Client certificate passed as string",
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSXT_CLIENT_AUTH_CERT", nil),
			},
			"client_auth_key": {
				Type:        schema.TypeString,
				Description: "Client certificate key passed as string",
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSXT_CLIENT_AUTH_KEY", nil),
			},
			"ca": {
				Type:        schema.TypeString,
				Description: "CA certificate passed as string",
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("NSXT_CA", nil),
			},
			"on_demand_connection": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "Avoid initializing NSX connection on startup",
				DefaultFunc: schema.EnvDefaultFunc("NSXT_ON_DEMAND_CONNECTION", false),
			},
		},

		DataSourcesMap: map[string]*schema.Resource{
			"nsxt_provider_info":                    dataSourceNsxtProviderInfo(),
			"nsxt_transport_zone":                   dataSourceNsxtTransportZone(),
			"nsxt_switching_profile":                dataSourceNsxtSwitchingProfile(),
			"nsxt_logical_tier0_router":             dataSourceNsxtLogicalTier0Router(),
			"nsxt_logical_tier1_router":             dataSourceNsxtLogicalTier1Router(),
			"nsxt_mac_pool":                         dataSourceNsxtMacPool(),
			"nsxt_ns_group":                         dataSourceNsxtNsGroup(),
			"nsxt_ns_groups":                        dataSourceNsxtNsGroups(),
			"nsxt_ns_service":                       dataSourceNsxtNsService(),
			"nsxt_ns_services":                      dataSourceNsxtNsServices(),
			"nsxt_edge_cluster":                     dataSourceNsxtEdgeCluster(),
			"nsxt_certificate":                      dataSourceNsxtCertificate(),
			"nsxt_ip_pool":                          dataSourceNsxtIPPool(),
			"nsxt_firewall_section":                 dataSourceNsxtFirewallSection(),
			"nsxt_management_cluster":               dataSourceNsxtManagementCluster(),
			"nsxt_policy_edge_cluster":              dataSourceNsxtPolicyEdgeCluster(),
			"nsxt_policy_edge_node":                 dataSourceNsxtPolicyEdgeNode(),
			"nsxt_policy_tier0_gateway":             dataSourceNsxtPolicyTier0Gateway(),
			"nsxt_policy_tier1_gateway":             dataSourceNsxtPolicyTier1Gateway(),
			"nsxt_policy_service":                   dataSourceNsxtPolicyService(),
			"nsxt_policy_realization_info":          dataSourceNsxtPolicyRealizationInfo(),
			"nsxt_policy_segment_realization":       dataSourceNsxtPolicySegmentRealization(),
			"nsxt_policy_transport_zone":            dataSourceNsxtPolicyTransportZone(),
			"nsxt_policy_ip_discovery_profile":      dataSourceNsxtPolicyIPDiscoveryProfile(),
			"nsxt_policy_spoofguard_profile":        dataSourceNsxtPolicySpoofGuardProfile(),
			"nsxt_policy_qos_profile":               dataSourceNsxtPolicyQosProfile(),
			"nsxt_policy_ipv6_ndra_profile":         dataSourceNsxtPolicyIpv6NdraProfile(),
			"nsxt_policy_ipv6_dad_profile":          dataSourceNsxtPolicyIpv6DadProfile(),
			"nsxt_policy_gateway_qos_profile":       dataSourceNsxtPolicyGatewayQosProfile(),
			"nsxt_policy_segment_security_profile":  dataSourceNsxtPolicySegmentSecurityProfile(),
			"nsxt_policy_mac_discovery_profile":     dataSourceNsxtPolicyMacDiscoveryProfile(),
			"nsxt_policy_vm":                        dataSourceNsxtPolicyVM(),
			"nsxt_policy_vms":                       dataSourceNsxtPolicyVMs(),
			"nsxt_policy_lb_app_profile":            dataSourceNsxtPolicyLBAppProfile(),
			"nsxt_policy_lb_client_ssl_profile":     dataSourceNsxtPolicyLBClientSslProfile(),
			"nsxt_policy_lb_server_ssl_profile":     dataSourceNsxtPolicyLBServerSslProfile(),
			"nsxt_policy_lb_monitor":                dataSourceNsxtPolicyLBMonitor(),
			"nsxt_policy_certificate":               dataSourceNsxtPolicyCertificate(),
			"nsxt_policy_lb_persistence_profile":    dataSourceNsxtPolicyLbPersistenceProfile(),
			"nsxt_policy_vni_pool":                  dataSourceNsxtPolicyVniPool(),
			"nsxt_policy_ip_block":                  dataSourceNsxtPolicyIPBlock(),
			"nsxt_policy_ip_pool":                   dataSourceNsxtPolicyIPPool(),
			"nsxt_policy_site":                      dataSourceNsxtPolicySite(),
			"nsxt_policy_gateway_policy":            dataSourceNsxtPolicyGatewayPolicy(),
			"nsxt_policy_security_policy":           dataSourceNsxtPolicySecurityPolicy(),
			"nsxt_policy_group":                     dataSourceNsxtPolicyGroup(),
			"nsxt_policy_context_profile":           dataSourceNsxtPolicyContextProfile(),
			"nsxt_policy_dhcp_server":               dataSourceNsxtPolicyDhcpServer(),
			"nsxt_policy_bfd_profile":               dataSourceNsxtPolicyBfdProfile(),
			"nsxt_policy_intrusion_service_profile": dataSourceNsxtPolicyIntrusionServiceProfile(),
			"nsxt_policy_lb_service":                dataSourceNsxtPolicyLbService(),
			"nsxt_policy_gateway_locale_service":    dataSourceNsxtPolicyGatewayLocaleService(),
			"nsxt_policy_bridge_profile":            dataSourceNsxtPolicyBridgeProfile(),
			"nsxt_policy_ipsec_vpn_local_endpoint":  dataSourceNsxtPolicyIPSecVpnLocalEndpoint(),
			"nsxt_policy_ipsec_vpn_service":         dataSourceNsxtPolicyIPSecVpnService(),
			"nsxt_policy_l2_vpn_service":            dataSourceNsxtPolicyL2VpnService(),
			"nsxt_policy_segment":                   dataSourceNsxtPolicySegment(),
			"nsxt_policy_project":                   dataSourceNsxtPolicyProject(),
			"nsxt_policy_gateway_prefix_list":       dataSourceNsxtPolicyGatewayPrefixList(),
			"nsxt_policy_gateway_route_map":         dataSourceNsxtPolicyGatewayRouteMap(),
			"nsxt_uplink_host_switch_profile":       dataSourceNsxtUplinkHostSwitchProfile(),
			"nsxt_compute_manager":                  dataSourceNsxtComputeManager(),
			"nsxt_transport_node_realization":       dataSourceNsxtTransportNodeRealization(),
			"nsxt_failure_domain":                   dataSourceNsxtFailureDomain(),
			"nsxt_compute_collection":               dataSourceNsxtComputeCollection(),
		},

		ResourcesMap: map[string]*schema.Resource{
			"nsxt_dhcp_relay_profile":                      resourceNsxtDhcpRelayProfile(),
			"nsxt_dhcp_relay_service":                      resourceNsxtDhcpRelayService(),
			"nsxt_dhcp_server_profile":                     resourceNsxtDhcpServerProfile(),
			"nsxt_logical_dhcp_server":                     resourceNsxtLogicalDhcpServer(),
			"nsxt_dhcp_server_ip_pool":                     resourceNsxtDhcpServerIPPool(),
			"nsxt_logical_switch":                          resourceNsxtLogicalSwitch(),
			"nsxt_vlan_logical_switch":                     resourceNsxtVlanLogicalSwitch(),
			"nsxt_logical_dhcp_port":                       resourceNsxtLogicalDhcpPort(),
			"nsxt_logical_port":                            resourceNsxtLogicalPort(),
			"nsxt_logical_tier0_router":                    resourceNsxtLogicalTier0Router(),
			"nsxt_logical_tier1_router":                    resourceNsxtLogicalTier1Router(),
			"nsxt_logical_router_centralized_service_port": resourceNsxtLogicalRouterCentralizedServicePort(),
			"nsxt_logical_router_downlink_port":            resourceNsxtLogicalRouterDownLinkPort(),
			"nsxt_logical_router_link_port_on_tier0":       resourceNsxtLogicalRouterLinkPortOnTier0(),
			"nsxt_logical_router_link_port_on_tier1":       resourceNsxtLogicalRouterLinkPortOnTier1(),
			"nsxt_ip_discovery_switching_profile":          resourceNsxtIPDiscoverySwitchingProfile(),
			"nsxt_mac_management_switching_profile":        resourceNsxtMacManagementSwitchingProfile(),
			"nsxt_qos_switching_profile":                   resourceNsxtQosSwitchingProfile(),
			"nsxt_spoofguard_switching_profile":            resourceNsxtSpoofGuardSwitchingProfile(),
			"nsxt_switch_security_switching_profile":       resourceNsxtSwitchSecuritySwitchingProfile(),
			"nsxt_l4_port_set_ns_service":                  resourceNsxtL4PortSetNsService(),
			"nsxt_algorithm_type_ns_service":               resourceNsxtAlgorithmTypeNsService(),
			"nsxt_icmp_type_ns_service":                    resourceNsxtIcmpTypeNsService(),
			"nsxt_igmp_type_ns_service":                    resourceNsxtIgmpTypeNsService(),
			"nsxt_ether_type_ns_service":                   resourceNsxtEtherTypeNsService(),
			"nsxt_ip_protocol_ns_service":                  resourceNsxtIPProtocolNsService(),
			"nsxt_ns_service_group":                        resourceNsxtNsServiceGroup(),
			"nsxt_ns_group":                                resourceNsxtNsGroup(),
			"nsxt_firewall_section":                        resourceNsxtFirewallSection(),
			"nsxt_nat_rule":                                resourceNsxtNatRule(),
			"nsxt_ip_block":                                resourceNsxtIPBlock(),
			"nsxt_ip_block_subnet":                         resourceNsxtIPBlockSubnet(),
			"nsxt_ip_pool":                                 resourceNsxtIPPool(),
			"nsxt_ip_pool_allocation_ip_address":           resourceNsxtIPPoolAllocationIPAddress(),
			"nsxt_ip_set":                                  resourceNsxtIPSet(),
			"nsxt_static_route":                            resourceNsxtStaticRoute(),
			"nsxt_vm_tags":                                 resourceNsxtVMTags(),
			"nsxt_lb_icmp_monitor":                         resourceNsxtLbIcmpMonitor(),
			"nsxt_lb_tcp_monitor":                          resourceNsxtLbTCPMonitor(),
			"nsxt_lb_udp_monitor":                          resourceNsxtLbUDPMonitor(),
			"nsxt_lb_http_monitor":                         resourceNsxtLbHTTPMonitor(),
			"nsxt_lb_https_monitor":                        resourceNsxtLbHTTPSMonitor(),
			"nsxt_lb_passive_monitor":                      resourceNsxtLbPassiveMonitor(),
			"nsxt_lb_pool":                                 resourceNsxtLbPool(),
			"nsxt_lb_tcp_virtual_server":                   resourceNsxtLbTCPVirtualServer(),
			"nsxt_lb_udp_virtual_server":                   resourceNsxtLbUDPVirtualServer(),
			"nsxt_lb_http_virtual_server":                  resourceNsxtLbHTTPVirtualServer(),
			"nsxt_lb_http_forwarding_rule":                 resourceNsxtLbHTTPForwardingRule(),
			"nsxt_lb_http_request_rewrite_rule":            resourceNsxtLbHTTPRequestRewriteRule(),
			"nsxt_lb_http_response_rewrite_rule":           resourceNsxtLbHTTPResponseRewriteRule(),
			"nsxt_lb_cookie_persistence_profile":           resourceNsxtLbCookiePersistenceProfile(),
			"nsxt_lb_source_ip_persistence_profile":        resourceNsxtLbSourceIPPersistenceProfile(),
			"nsxt_lb_client_ssl_profile":                   resourceNsxtLbClientSslProfile(),
			"nsxt_lb_server_ssl_profile":                   resourceNsxtLbServerSslProfile(),
			"nsxt_lb_service":                              resourceNsxtLbService(),
			"nsxt_lb_fast_tcp_application_profile":         resourceNsxtLbFastTCPApplicationProfile(),
			"nsxt_lb_fast_udp_application_profile":         resourceNsxtLbFastUDPApplicationProfile(),
			"nsxt_lb_http_application_profile":             resourceNsxtLbHTTPApplicationProfile(),
			"nsxt_policy_tier1_gateway":                    resourceNsxtPolicyTier1Gateway(),
			"nsxt_policy_tier1_gateway_interface":          resourceNsxtPolicyTier1GatewayInterface(),
			"nsxt_policy_tier0_gateway":                    resourceNsxtPolicyTier0Gateway(),
			"nsxt_policy_tier0_gateway_interface":          resourceNsxtPolicyTier0GatewayInterface(),
			"nsxt_policy_tier0_gateway_ha_vip_config":      resourceNsxtPolicyTier0GatewayHAVipConfig(),
			"nsxt_policy_group":                            resourceNsxtPolicyGroup(),
			"nsxt_policy_domain":                           resourceNsxtPolicyDomain(),
			"nsxt_policy_security_policy":                  resourceNsxtPolicySecurityPolicy(),
			"nsxt_policy_service":                          resourceNsxtPolicyService(),
			"nsxt_policy_gateway_policy":                   resourceNsxtPolicyGatewayPolicy(),
			"nsxt_policy_predefined_gateway_policy":        resourceNsxtPolicyPredefinedGatewayPolicy(),
			"nsxt_policy_predefined_security_policy":       resourceNsxtPolicyPredefinedSecurityPolicy(),
			"nsxt_policy_segment":                          resourceNsxtPolicySegment(),
			"nsxt_policy_vlan_segment":                     resourceNsxtPolicyVlanSegment(),
			"nsxt_policy_fixed_segment":                    resourceNsxtPolicyFixedSegment(),
			"nsxt_policy_static_route":                     resourceNsxtPolicyStaticRoute(),
			"nsxt_policy_gateway_prefix_list":              resourceNsxtPolicyGatewayPrefixList(),
			"nsxt_policy_vm_tags":                          resourceNsxtPolicyVMTags(),
			"nsxt_policy_nat_rule":                         resourceNsxtPolicyNATRule(),
			"nsxt_policy_ip_block":                         resourceNsxtPolicyIPBlock(),
			"nsxt_policy_lb_pool":                          resourceNsxtPolicyLBPool(),
			"nsxt_policy_ip_pool":                          resourceNsxtPolicyIPPool(),
			"nsxt_policy_ip_pool_block_subnet":             resourceNsxtPolicyIPPoolBlockSubnet(),
			"nsxt_policy_ip_pool_static_subnet":            resourceNsxtPolicyIPPoolStaticSubnet(),
			"nsxt_policy_lb_service":                       resourceNsxtPolicyLBService(),
			"nsxt_policy_lb_virtual_server":                resourceNsxtPolicyLBVirtualServer(),
			"nsxt_policy_ip_address_allocation":            resourceNsxtPolicyIPAddressAllocation(),
			"nsxt_policy_bgp_neighbor":                     resourceNsxtPolicyBgpNeighbor(),
			"nsxt_policy_bgp_config":                       resourceNsxtPolicyBgpConfig(),
			"nsxt_policy_dhcp_relay":                       resourceNsxtPolicyDhcpRelayConfig(),
			"nsxt_policy_dhcp_server":                      resourceNsxtPolicyDhcpServer(),
			"nsxt_policy_context_profile":                  resourceNsxtPolicyContextProfile(),
			"nsxt_policy_dhcp_v4_static_binding":           resourceNsxtPolicyDhcpV4StaticBinding(),
			"nsxt_policy_dhcp_v6_static_binding":           resourceNsxtPolicyDhcpV6StaticBinding(),
			"nsxt_policy_dns_forwarder_zone":               resourceNsxtPolicyDNSForwarderZone(),
			"nsxt_policy_gateway_dns_forwarder":            resourceNsxtPolicyGatewayDNSForwarder(),
			"nsxt_policy_gateway_community_list":           resourceNsxtPolicyGatewayCommunityList(),
			"nsxt_policy_gateway_route_map":                resourceNsxtPolicyGatewayRouteMap(),
			"nsxt_policy_intrusion_service_policy":         resourceNsxtPolicyIntrusionServicePolicy(),
			"nsxt_policy_static_route_bfd_peer":            resourceNsxtPolicyStaticRouteBfdPeer(),
			"nsxt_policy_intrusion_service_profile":        resourceNsxtPolicyIntrusionServiceProfile(),
			"nsxt_policy_evpn_tenant":                      resourceNsxtPolicyEvpnTenant(),
			"nsxt_policy_evpn_config":                      resourceNsxtPolicyEvpnConfig(),
			"nsxt_policy_evpn_tunnel_endpoint":             resourceNsxtPolicyEvpnTunnelEndpoint(),
			"nsxt_policy_vni_pool":                         resourceNsxtPolicyVniPool(),
			"nsxt_policy_qos_profile":                      resourceNsxtPolicyQosProfile(),
			"nsxt_policy_ospf_config":                      resourceNsxtPolicyOspfConfig(),
			"nsxt_policy_ospf_area":                        resourceNsxtPolicyOspfArea(),
			"nsxt_policy_gateway_redistribution_config":    resourceNsxtPolicyGatewayRedistributionConfig(),
			"nsxt_policy_mac_discovery_profile":            resourceNsxtPolicyMacDiscoveryProfile(),
			"nsxt_policy_ipsec_vpn_ike_profile":            resourceNsxtPolicyIPSecVpnIkeProfile(),
			"nsxt_policy_ipsec_vpn_tunnel_profile":         resourceNsxtPolicyIPSecVpnTunnelProfile(),
			"nsxt_policy_ipsec_vpn_dpd_profile":            resourceNsxtPolicyIPSecVpnDpdProfile(),
			"nsxt_policy_ipsec_vpn_session":                resourceNsxtPolicyIPSecVpnSession(),
			"nsxt_policy_l2_vpn_session":                   resourceNsxtPolicyL2VPNSession(),
			"nsxt_policy_ipsec_vpn_service":                resourceNsxtPolicyIPSecVpnService(),
			"nsxt_policy_l2_vpn_service":                   resourceNsxtPolicyL2VpnService(),
			"nsxt_policy_ipsec_vpn_local_endpoint":         resourceNsxtPolicyIPSecVpnLocalEndpoint(),
			"nsxt_policy_ip_discovery_profile":             resourceNsxtPolicyIPDiscoveryProfile(),
			"nsxt_policy_context_profile_custom_attribute": resourceNsxtPolicyContextProfileCustomAttribute(),
			"nsxt_policy_segment_security_profile":         resourceNsxtPolicySegmentSecurityProfile(),
			"nsxt_policy_spoof_guard_profile":              resourceNsxtPolicySpoofGuardProfile(),
			"nsxt_policy_gateway_qos_profile":              resourceNsxtPolicyGatewayQosProfile(),
			"nsxt_policy_project":                          resourceNsxtPolicyProject(),
			"nsxt_policy_transport_zone":                   resourceNsxtPolicyTransportZone(),
			"nsxt_edge_cluster":                            resourceNsxtEdgeCluster(),
			"nsxt_compute_manager":                         resourceNsxtComputeManager(),
			"nsxt_manager_cluster":                         resourceNsxtManagerCluster(),
			"nsxt_uplink_host_switch_profile":              resourceNsxtUplinkHostSwitchProfile(),
			"nsxt_transport_node":                          resourceNsxtTransportNode(),
			"nsxt_failure_domain":                          resourceNsxtFailureDomain(),
			"nsxt_cluster_virtual_ip":                      resourceNsxtClusterVirualIP(),
			"nsxt_policy_host_transport_node_profile":      resourceNsxtPolicyHostTransportNodeProfile(),
		},

		ConfigureFunc: providerConfigure,
	}
}

func configureNsxtClient(d *schema.ResourceData, clients *nsxtClients) error {
	onDemandConn := d.Get("on_demand_connection").(bool)
	clientAuthCertFile := d.Get("client_auth_cert_file").(string)
	clientAuthKeyFile := d.Get("client_auth_key_file").(string)
	clientAuthCert := d.Get("client_auth_cert").(string)
	clientAuthKey := d.Get("client_auth_key").(string)
	vmcToken := d.Get("vmc_token").(string)
	vmcAuthMode := d.Get("vmc_auth_mode").(string)

	if onDemandConn {
		// On demand connection option is not supported with old SDK
		return nil
	}

	if (len(vmcToken) > 0) || (vmcAuthMode == "Basic") {
		// VMC can operate without token with basic auth, however MP API is not
		// available for cloud admin user
		return nil
	}

	needCreds := true
	if len(clientAuthCertFile) > 0 {
		if len(clientAuthKeyFile) == 0 {
			return fmt.Errorf("Please provide key file for client certificate")
		}
		needCreds = false
	}

	if len(clientAuthCert) > 0 {
		if len(clientAuthKey) == 0 {
			return fmt.Errorf("Please provide key for client certificate")
		}
		// only supported for policy resources
		needCreds = false
	}

	insecure := d.Get("allow_unverified_ssl").(bool)
	username := d.Get("username").(string)
	password := d.Get("password").(string)

	if needCreds {
		if username == "" {
			return fmt.Errorf("username must be provided")
		}

		if password == "" {
			return fmt.Errorf("password must be provided")
		}
	}

	host := d.Get("host").(string)
	// Remove schema
	host = strings.TrimPrefix(host, "https://")

	if host == "" {
		return fmt.Errorf("host must be provided")
	}

	caFile := d.Get("ca_file").(string)
	caString := d.Get("ca").(string)
	sessionAuth := d.Get("session_auth").(bool)
	skipSessionAuth := !sessionAuth

	retriesConfig := api.ClientRetriesConfiguration{
		MaxRetries:      clients.CommonConfig.MaxRetries,
		RetryMinDelay:   clients.CommonConfig.MinRetryInterval,
		RetryMaxDelay:   clients.CommonConfig.MaxRetryInterval,
		RetryOnStatuses: clients.CommonConfig.RetryStatusCodes,
	}

	clients.NsxtClientConfig = &api.Configuration{
		BasePath:             "/api/v1",
		Host:                 host,
		Scheme:               "https",
		UserAgent:            "terraform-provider-nsxt",
		UserName:             username,
		Password:             password,
		RemoteAuth:           clients.CommonConfig.RemoteAuth,
		ClientAuthCertFile:   clientAuthCertFile,
		ClientAuthKeyFile:    clientAuthKeyFile,
		CAFile:               caFile,
		ClientAuthCertString: clientAuthCert,
		ClientAuthKeyString:  clientAuthKey,
		CAString:             caString,
		Insecure:             insecure,
		RetriesConfiguration: retriesConfig,
		SkipSessionAuth:      skipSessionAuth,
	}

	nsxClient, err := api.NewAPIClient(clients.NsxtClientConfig)
	if err != nil {
		return err
	}

	clients.NsxtClient = nsxClient

	return nil
}

type jwtToken struct {
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func getAPIToken(vmcAuthHost string, vmcAccessToken string) (string, error) {

	payload := strings.NewReader("refresh_token=" + vmcAccessToken)
	req, _ := http.NewRequest("POST", "https://"+vmcAuthHost, payload)

	req.Header.Add("content-type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)

	if err != nil {
		return "", err
	}

	if res.StatusCode != 200 {
		b, _ := ioutil.ReadAll(res.Body)
		return "", fmt.Errorf("Unexpected status code %d trying to get auth token. %s", res.StatusCode, string(b))
	}

	defer res.Body.Close()
	token := jwtToken{}
	err = json.NewDecoder(res.Body).Decode(&token)
	if err != nil {
		// Not fatal
		log.Printf("[WARNING]: Failed to decode access token from response: %v", err)
	}

	return token.AccessToken, nil
}

func getConnectorTLSConfig(d *schema.ResourceData) (*tls.Config, error) {

	insecure := d.Get("allow_unverified_ssl").(bool)
	clientAuthCertFile := d.Get("client_auth_cert_file").(string)
	clientAuthKeyFile := d.Get("client_auth_key_file").(string)
	caFile := d.Get("ca_file").(string)
	clientAuthCert := d.Get("client_auth_cert").(string)
	clientAuthKey := d.Get("client_auth_key").(string)
	caCert := d.Get("ca").(string)
	tlsConfig := tls.Config{InsecureSkipVerify: insecure}

	if len(clientAuthCertFile) > 0 {

		// cert and key are passed via filesystem
		if len(clientAuthKeyFile) == 0 {
			return nil, fmt.Errorf("Please provide key file for client certificate")
		}

		cert, err := tls.LoadX509KeyPair(clientAuthCertFile, clientAuthKeyFile)

		if err != nil {
			return nil, fmt.Errorf("Failed to load client cert/key pair: %v", err)
		}

		tlsConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &cert, nil
		}
	}

	if len(clientAuthCert) > 0 {
		// cert and key are passed as strings
		if len(clientAuthKey) == 0 {
			return nil, fmt.Errorf("Please provide key for client certificate")
		}

		cert, err := tls.X509KeyPair([]byte(clientAuthCert), []byte(clientAuthKey))

		if err != nil {
			return nil, fmt.Errorf("Failed to load client cert/key pair: %v", err)
		}

		tlsConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &cert, nil
		}
	}

	if len(caFile) > 0 {
		caCert, err := ioutil.ReadFile(caFile)
		if err != nil {
			return nil, err
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		tlsConfig.RootCAs = caCertPool
	}

	if len(caCert) > 0 {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM([]byte(caCert))

		tlsConfig.RootCAs = caCertPool
	}

	return &tlsConfig, nil
}

func configurePolicyConnectorData(d *schema.ResourceData, clients *nsxtClients) error {
	onDemandConn := d.Get("on_demand_connection").(bool)
	host := d.Get("host").(string)
	username := d.Get("username").(string)
	password := d.Get("password").(string)
	vmcAccessToken := d.Get("vmc_token").(string)
	vmcAuthHost := d.Get("vmc_auth_host").(string)
	clientAuthCertFile := d.Get("client_auth_cert_file").(string)
	clientAuthCert := d.Get("client_auth_cert").(string)
	clientAuthDefined := (len(clientAuthCertFile) > 0) || (len(clientAuthCert) > 0)
	policyEnforcementPoint := d.Get("enforcement_point").(string)
	policyGlobalManager := d.Get("global_manager").(bool)
	vmcAuthMode := d.Get("vmc_auth_mode").(string)

	isVMC := false
	if (len(vmcAccessToken) > 0) || (vmcAuthMode == "Basic") {
		isVMC = true
		if onDemandConn {
			return fmt.Errorf("on demand connection option is not supported with VMC")
		}
	}

	if host == "" {
		return fmt.Errorf("host must be provided")
	}

	if !strings.HasPrefix(host, "https://") {
		host = fmt.Sprintf("https://%s", host)
	}

	securityContextNeeded := true
	if clientAuthDefined && !clients.CommonConfig.RemoteAuth {
		securityContextNeeded = false
	}
	if securityContextNeeded {
		securityCtx, err := getConfiguredSecurityContext(clients, vmcAccessToken, vmcAuthHost, vmcAuthMode, username, password)
		if err != nil {
			return err
		}
		clients.PolicySecurityContext = securityCtx
	}

	tlsConfig, err := getConnectorTLSConfig(d)
	if err != nil {
		return err
	}

	tr := &http.Transport{
		Proxy:           http.ProxyFromEnvironment,
		TLSClientConfig: tlsConfig,
	}

	httpClient := http.Client{Transport: tr}
	clients.PolicyHTTPClient = &httpClient
	clients.Host = host
	clients.PolicyEnforcementPoint = policyEnforcementPoint
	clients.PolicyGlobalManager = policyGlobalManager

	if onDemandConn {
		// version init will happen on demand
		return nil
	}

	if !isVMC {
		err = configureLicenses(getPolicyConnectorForInit(*clients, true), clients.CommonConfig.LicenseDiff)
		if err != nil {
			return err
		}
	}

	err = initNSXVersion(getPolicyConnectorForInit(*clients, true))
	if err != nil && isVMC {
		// In case version API does not work for VMC, we workaround by testing version-specific APIs
		// TODO - remove this when /node/version API works for all auth methods on VMC
		initNSXVersionVMC(*clients)
		return nil
	}
	return err
}

func getConfiguredSecurityContext(clients *nsxtClients, vmcAccessToken string, vmcAuthHost string, vmcAuthMode string, username string, password string) (*core.SecurityContextImpl, error) {
	securityCtx := core.NewSecurityContextImpl()
	if len(vmcAccessToken) > 0 {
		if vmcAuthHost == "" {
			return nil, fmt.Errorf("vmc auth host must be provided if auth token is provided")
		}

		apiToken, err := getAPIToken(vmcAuthHost, vmcAccessToken)
		if err != nil {
			return nil, err
		}

		// We'll be sending Bearer token anyway even with scp-auth-token auth
		// For now, node API is not working on VMC without Bearer token present
		clients.CommonConfig.BearerToken = apiToken
		if vmcAuthMode != "Bearer" {
			securityCtx.SetProperty(security.AUTHENTICATION_SCHEME_ID, security.OAUTH_SCHEME_ID)
			securityCtx.SetProperty(security.ACCESS_TOKEN, apiToken)
		}
	} else {
		if username == "" {
			return nil, fmt.Errorf("username must be provided")
		}

		if password == "" {
			return nil, fmt.Errorf("password must be provided")
		}

		securityCtx.SetProperty(security.AUTHENTICATION_SCHEME_ID, security.USER_PASSWORD_SCHEME_ID)
		securityCtx.SetProperty(security.USER_KEY, username)
		securityCtx.SetProperty(security.PASSWORD_KEY, password)
	}
	return securityCtx, nil
}

type customHeaderProcessor struct {
	customHeaders *map[string]string
}

func (processor customHeaderProcessor) Process(req *http.Request) error {
	for header, value := range *processor.customHeaders {
		req.Header.Set(header, value)
	}
	return nil
}

func newCustomHeaderProcessor(customHeaders *map[string]string) *customHeaderProcessor {
	return &customHeaderProcessor{customHeaders: customHeaders}
}

type remoteAuthHeaderProcessor struct {
}

func newRemoteAuthHeaderProcessor() *remoteAuthHeaderProcessor {
	return &remoteAuthHeaderProcessor{}
}

func (processor remoteAuthHeaderProcessor) Process(req *http.Request) error {
	oldAuthHeader := req.Header.Get("Authorization")
	newAuthHeader := strings.Replace(oldAuthHeader, "Basic", "Remote", 1)
	req.Header.Set("Authorization", newAuthHeader)
	return nil
}

type logRequestProcessor struct {
}

func newLogRequestProcessor() *logRequestProcessor {
	return &logRequestProcessor{}
}

func (processor logRequestProcessor) Process(req *http.Request) error {
	reqDump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Issuing request towards NSX:\n%s", reqDump)
	return nil
}

type logResponseAcceptor struct {
}

func newLogResponseAcceptor() *logResponseAcceptor {
	return &logResponseAcceptor{}
}

func (processor logResponseAcceptor) Accept(req *http.Response) {
	dumpResponse, err := httputil.DumpResponse(req, true)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Received NSX response:\n%s", dumpResponse)
}

type bearerAuthHeaderProcessor struct {
	Token string
}

func newBearerAuthHeaderProcessor(token string) *bearerAuthHeaderProcessor {
	return &bearerAuthHeaderProcessor{Token: token}
}

func (processor bearerAuthHeaderProcessor) Process(req *http.Request) error {
	newAuthHeader := fmt.Sprintf("Bearer %s", processor.Token)
	req.Header.Set("Authorization", newAuthHeader)
	return nil
}

type sessionHeaderProcessor struct {
	cookie string
	xsrf   string
}

func newSessionHeaderProcessor(cookie string, xsrf string) *sessionHeaderProcessor {
	return &sessionHeaderProcessor{
		cookie: cookie,
		xsrf:   xsrf,
	}
}

func (processor sessionHeaderProcessor) Process(req *http.Request) error {
	req.Header.Set("Cookie", processor.cookie)
	req.Header.Set("X-XSRF-TOKEN", processor.xsrf)
	return nil
}

func applyLicense(connector client.Connector, licenseKey string) error {
	client := nsx.NewLicensesClient(connector)
	license := model.License{LicenseKey: &licenseKey}
	_, err := client.Create(license)
	if err != nil {
		return fmt.Errorf("Error during license create: %v", err)
	}

	return nil
}

func removeLicense(connector client.Connector, licenseKey string) error {
	client := nsx.NewLicensesClient(connector)
	err := client.Delete(licenseKey)
	if err != nil {
		return fmt.Errorf("error during license delete: %v", err)
	}

	return nil
}

func getListDiffFromSchema(d *schema.ResourceData, attribute string) listDiff {
	var diff listDiff
	o, n := d.GetChange(attribute)
	oldValues := interfaceListToStringList(o.([]interface{}))
	newValues := interfaceListToStringList(n.([]interface{}))
	// Check for new
	for _, value := range newValues {
		if !slices.Contains(oldValues, value) {
			diff.added = append(diff.added, value)
		}
	}

	// Remove old licenses
	for _, value := range oldValues {
		if !slices.Contains(newValues, value) {
			diff.removed = append(diff.removed, value)
		}
	}
	return diff
}

// license keys are applied on terraform plan and are not removed
func configureLicenses(connector client.Connector, diff listDiff) error {
	// Apply new licenses
	for _, license := range diff.added {
		err := applyLicense(connector, license.(string))
		if err != nil {
			return fmt.Errorf("error applying license key: %s. %s", license, err.Error())
		}
	}

	// Remove old licenses
	for _, license := range diff.removed {
		err := removeLicense(connector, license.(string))
		if err != nil {
			return fmt.Errorf("error deleting license key: %s. %s", license, err.Error())
		}
	}
	return nil
}

func initCommonConfig(d *schema.ResourceData) commonProviderConfig {
	remoteAuth := d.Get("remote_auth").(bool)
	toleratePartialSuccess := d.Get("tolerate_partial_success").(bool)
	maxRetries := d.Get("max_retries").(int)
	retryMinDelay := d.Get("retry_min_delay").(int)
	retryMaxDelay := d.Get("retry_max_delay").(int)
	username := d.Get("username").(string)
	password := d.Get("password").(string)

	statuses := d.Get("retry_on_status_codes").([]interface{})
	retryStatuses := make([]int, 0, len(statuses))
	for _, s := range statuses {
		retryStatuses = append(retryStatuses, s.(int))
	}

	if len(retryStatuses) == 0 {
		// Set to the defaults if empty
		retryStatuses = append(retryStatuses, defaultRetryOnStatusCodes...)
	}

	licDiff := getListDiffFromSchema(d, "license_keys")
	return commonProviderConfig{
		RemoteAuth:             remoteAuth,
		ToleratePartialSuccess: toleratePartialSuccess,
		MaxRetries:             maxRetries,
		MinRetryInterval:       retryMinDelay,
		MaxRetryInterval:       retryMaxDelay,
		RetryStatusCodes:       retryStatuses,
		Username:               username,
		Password:               password,
		LicenseDiff:            licDiff,
	}
}

func providerConfigure(d *schema.ResourceData) (interface{}, error) {
	commonConfig := initCommonConfig(d)
	clients := nsxtClients{
		CommonConfig: commonConfig,
	}

	err := configureNsxtClient(d, &clients)
	if err != nil {
		return nil, err
	}

	err = configurePolicyConnectorData(d, &clients)
	if err != nil {
		return nil, err
	}

	return clients, nil
}

func getPolicyConnector(clients interface{}) client.Connector {
	return getPolicyConnectorWithHeaders(clients, nil, false, true)
}

func getPolicyConnectorForInit(clients interface{}, withRetry bool) client.Connector {
	return getPolicyConnectorWithHeaders(clients, nil, true, withRetry)
}

func getPolicyConnectorWithHeaders(clients interface{}, customHeaders *map[string]string, initFlow bool, withRetry bool) client.Connector {
	c := clients.(nsxtClients)

	retryFunc := func(retryContext retry.RetryContext) bool {
		shouldRetry := false
		if retryContext.Response != nil {
			for _, code := range c.CommonConfig.RetryStatusCodes {
				if retryContext.Response.StatusCode == code {
					log.Printf("[DEBUG]: Retrying request due to error code %d", code)
					shouldRetry = true
					break
				}
			}
		} else {
			shouldRetry = true
			log.Printf("[DEBUG]: Retrying request due to error")
		}

		if !shouldRetry {
			return false
		}

		min := c.CommonConfig.MinRetryInterval
		max := c.CommonConfig.MaxRetryInterval
		if max > 0 {
			interval := (rand.Intn(max-min) + min)
			time.Sleep(time.Duration(interval) * time.Millisecond)
			log.Printf("[DEBUG]: Waited %d ms before retrying", interval)
		}

		return true
	}

	connectorOptions := []client.ConnectorOption{client.UsingRest(nil), client.WithHttpClient(c.PolicyHTTPClient)}
	var requestProcessors []core.RequestProcessor
	var responseAcceptors []core.ResponseAcceptor

	if withRetry {
		connectorOptions = append(connectorOptions, client.WithDecorators(retry.NewRetryDecorator(uint(c.CommonConfig.MaxRetries), retryFunc)))
	}

	if c.PolicySecurityContext != nil {
		connectorOptions = append(connectorOptions, client.WithSecurityContext(c.PolicySecurityContext))
	}
	if c.CommonConfig.RemoteAuth {
		requestProcessors = append(requestProcessors, newRemoteAuthHeaderProcessor().Process)
	}
	if len(c.CommonConfig.BearerToken) > 0 {
		requestProcessors = append(requestProcessors, newBearerAuthHeaderProcessor(c.CommonConfig.BearerToken).Process)
	}
	if customHeaders != nil {
		requestProcessors = append(requestProcessors, newCustomHeaderProcessor(customHeaders).Process)
	}

	// Session support for policy resources (main rationale - vIDM environment where auth is slow)
	// Currently session creation is done via old MP sdk.
	// TODO - when MP resources are removed, switch to official SDK to initiate session/create API
	// TODO - re-trigger session/create when token is expired
	if c.NsxtClientConfig != nil && len(c.NsxtClientConfig.DefaultHeader["Cookie"]) > 0 {
		cookie := c.NsxtClientConfig.DefaultHeader["Cookie"]
		xsrf := ""
		if len(c.NsxtClientConfig.DefaultHeader["X-XSRF-TOKEN"]) > 0 {
			xsrf = c.NsxtClientConfig.DefaultHeader["X-XSRF-TOKEN"]
		}
		requestProcessors = append(requestProcessors, newSessionHeaderProcessor(cookie, xsrf).Process)
		log.Printf("[INFO]: Session headers configured for policy objects")
	}

	if os.Getenv("TF_LOG_PROVIDER_NSX_HTTP") != "" {
		requestProcessors = append(requestProcessors, newLogRequestProcessor().Process)
		responseAcceptors = append(responseAcceptors, newLogResponseAcceptor().Accept)
	}

	if len(requestProcessors) > 0 {
		connectorOptions = append(connectorOptions, client.WithRequestProcessors(requestProcessors...))
	}
	if len(responseAcceptors) > 0 {
		connectorOptions = append(connectorOptions, client.WithResponseAcceptors(responseAcceptors...))
	}
	connector := client.NewConnector(c.Host, connectorOptions...)
	// Init NSX version on demand if not done yet
	// This is also our indication to apply licenses, in case of delayed connection
	if nsxVersion == "" && !initFlow {
		initNSXVersion(connector)
		err := configureLicenses(connector, c.CommonConfig.LicenseDiff)
		if err != nil {
			log.Printf("[ERROR]: Failed to apply NSX licenses")
		}
	}
	return connector
}

func getPolicyEnforcementPoint(clients interface{}) string {
	return clients.(nsxtClients).PolicyEnforcementPoint
}

func isPolicyGlobalManager(clients interface{}) bool {
	return clients.(nsxtClients).PolicyGlobalManager
}

func getCommonProviderConfig(clients interface{}) commonProviderConfig {
	return clients.(nsxtClients).CommonConfig
}

func getGlobalPolicyEnforcementPointPath(m interface{}, sitePath *string) string {
	return fmt.Sprintf("%s/enforcement-points/%s", *sitePath, getPolicyEnforcementPoint(m))
}

func getProjectIDFromSchema(d *schema.ResourceData) string {
	ctxPtr := d.Get("context")
	if ctxPtr != nil {
		contexts := ctxPtr.([]interface{})
		for _, context := range contexts {
			data := context.(map[string]interface{})

			return data["project_id"].(string)
		}
	}
	return ""
}

func getSessionContext(d *schema.ResourceData, m interface{}) tf_api.SessionContext {
	var clientType tf_api.ClientType
	projectID := getProjectIDFromSchema(d)
	if projectID != "" {
		clientType = tf_api.Multitenancy
	} else if isPolicyGlobalManager(m) {
		clientType = tf_api.Global
	} else {
		clientType = tf_api.Local
	}
	return tf_api.SessionContext{ProjectID: projectID, ClientType: clientType}
}
