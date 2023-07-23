---
subcategory: "Policy - Multi Tenancy"
layout: "nsxt"
page_title: "NSXT: policy_vpc"
description: Policy VPC data source.
---

# nsxt_policy_vpc

This data source provides information about policy VPC configured on NSX.
This data source is applicable to NSX Policy Manager and VMC.

## Example Usage

```hcl
data "nsxt_policy_vpc" "test" {
  display_name = "vpc1"
}
```

## Argument Reference

* `id` - (Optional) The ID of VPC to retrieve. If ID is specified, no additional argument should be configured.
* `display_name` - (Optional) The Display Name prefix of the VPC to retrieve.

## Attributes Reference

In addition to arguments listed above, the following attributes are exported:

* `description` - The description of the resource.
* `path` - The NSX path of the policy resource.
* `short_id` - Defaults to id if id is less than equal to 8 characters or defaults to random generated id if not set.
* `site_info` - Information related to sites applicable for given VPC.
  * `edge_cluster_paths` - The edge cluster on which the networking elements for the Org will be created.
  * `site_path` - This represents the path of the site which is managed by Global Manager. For the local manager, if set, this needs to point to 'default'.
