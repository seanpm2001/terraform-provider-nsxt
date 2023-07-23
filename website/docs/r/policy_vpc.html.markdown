---
subcategory: "Policy - Multi Tenancy"
layout: "nsxt"
page_title: "NSXT: nsxt_policy_vpc"
description: A resource to configure a VPC.
---

# nsxt_policy_vpc

This resource provides a method for the management of a VPC.

This resource is applicable to NSX Global Manager, NSX Policy Manager and VMC.

## Example Usage

```hcl
resource "nsxt_policy_vpc" "test" {
  display_name        = "test"
  description         = "Terraform provisioned VPC"
  _default            = true
  short_id            = "test"
  ip_address_type     = "IPV4"
  default_gateway_path = "/infra/tier-0s/Tier0GatewayTest"
  external_ipv4_blocks = ["/infra/ip-blocks/ip_block_dev"]
  site_info {
    edge_cluster_paths = ["/infra/sites/default/enforcement-points/default/edge-clusters/10667080-06c7-46c3-9665-4f0364abce84"]
    site_path          = "/infra/sites/default"
  }
}
```

## Argument Reference

The following arguments are supported:

* `display_name` - (Required) Display name of the resource.
* `description` - (Optional) Description of the resource.
* `tag` - (Optional) A list of scope + tag pairs to associate with this resource.
* `nsx_id` - (Optional) The NSX ID of this resource. If set, this ID will be used to create the resource.
* `_default` - (Optional) The server will populate this field when returing the resource. Ignored on PUT and POST.true - the project is a default project. Default projects are non-editable, system create ones.
* `short_id` - (Optional) Defaults to id if id is less than equal to 8 characters or defaults to random generated id if not set.
* `site_info` - (Optional) Information related to sites applicable for given VPC. For on-prem deployment, only 1 is allowed.
  * `edge_cluster_paths` - (Optional) The edge cluster on which the networking elements for the VPC will be created.
  * `site_path` - (Optional) This represents the path of the site which is managed by NSX Manager. For the local manager, if set, this needs to point to 'default'.
* `default_gateway_path` - (Optional) PolicyPath of Tier0 or Tier0 VRF gateway or label path referencing to Tier0 or Tier0 VRF. In case of Label, it should have reference of Tier0 or Tier0 VRF path.
* `external_ipv4_blocks` - (Optional) PolicyPath of external IPv4 block used for allocating CIDR blocks for public subnets. IP block must be subset of Project IPv4 blocks.
* `ip_address_type` - (Required) The IP address type that will be allocated for subnets. In the case of IPv4, all the subnets will be allocated IP addresses from the IpV4 private/external pool.
* `ipv6_profile_paths` - (Optional) IPv6 NDRA and/or DAD profiles configuration. If not specified, default profiles will be applied.
* `private_ipv4_blocks` - (Optional) PolicyPath of private ip block used for allocating CIDR blocks for private subnets. IP block must be defined by the Project admin.
* `subnet_profiles` - (Optional) Subnet profiles used to create subnet profile binding. Subnet profiles need to be pre-created at the project level. If not specified, default profiles will be used.
  * `ip_discovery` - (Optional) IP Discovery Profile.
  * `mac_discovery` - (Optional) Mac Discovery Profile.
  * `qos` - (Optional) Segment Qos Profile.
  * `segment_security` - (Optional) Segment Security Profile.
  * `spoof_guard` - (Optional) SpoofGuard profile.
* `service_gateway` - (Optional) Service Gateway configuration.
  * `auto_snat` - (Optional) Auto plumb snat rule for private subnet, this will make sure private subnets are routable outside of VPC.
  * `disable` - (Optional) Flag to indicate if Gateway Service support is required or not. By default, service gateway is enabled.
  * `qos_config` - (Optional) QoS Profile configuration for VPC connected to the gateway. The profiles must be pre-created at the project level.
    * `egress_qos_profile_path` - (Optional) Policy path to gateway QoS profile in egress direction.
    * `ingress_qos_profile_path` - (Optional) Policy path to gateway QoS profile in ingress direction.
* `load_balancer_vpc_endpoint` - (Optional) Load Balancer configuration
  * `enabled` - (Optional) Flag to indicate whether support for load balancing is needed. Setting this flag to true causes allocation of private IPs from the private block associated with this VPC to be used by the load balancer.
* `dhcp_config` - (Optional) DHCP configuration.
  * `dhcp_relay_config_path` - (Optional) Policy path of DHCP-relay-config. If configured then all the subnets will be configured with the DHCP relay server. If not specified, then the local DHCP server will be configured for all connected subnets.
  * `enable_dhcp` - (Optional) Flag to enable or disable DHCP. If enabled, the DHCP server will be configured based on IP address type. If disabled then neither DHCP server nor relay shall be configured.
  * `dns_client_config` - (Optional) Dns client configuration.
    * `dns_server_ips` - (Optional) IPs of the DNS servers which need to be configured on teh workload VMs.

## Attributes Reference

In addition to arguments listed above, the following attributes are exported:

* `id` - ID of the resource.
* `revision` - Indicates current revision number of the object as seen by NSX-T API server. This attribute can be useful for debugging.
* `path` - The NSX path of the policy resource.

## Importing

An existing object can be [imported][docs-import] into this resource, via the following command:

[docs-import]: https://www.terraform.io/cli/import

```
terraform import nsxt_policy_vpc.test UUID
```

The above command imports VPC named `test` with the NSX ID `UUID`.
