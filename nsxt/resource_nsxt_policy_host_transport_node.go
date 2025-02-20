/* Copyright © 2020 VMware, Inc. All Rights Reserved.
   SPDX-License-Identifier: MPL-2.0 */

package nsxt

import (
	"fmt"
	"log"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/vmware/vsphere-automation-sdk-go/runtime/protocol/client"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt/infra"
	"github.com/vmware/vsphere-automation-sdk-go/services/nsxt/infra/sites/enforcement_points"
	model2 "github.com/vmware/vsphere-automation-sdk-go/services/nsxt/model"
	"golang.org/x/exp/maps"
)

func resourceNsxtPolicyHostTransportNode() *schema.Resource {
	return &schema.Resource{
		Create: resourceNsxtPolicyHostTransportNodeCreate,
		Read:   resourceNsxtPolicyHostTransportNodeRead,
		Update: resourceNsxtPolicyHostTransportNodeUpdate,
		Delete: resourceNsxtPolicyHostTransportNodeDelete,
		Importer: &schema.ResourceImporter{
			State: resourceNsxtPolicyHostTransportNodeImporter,
		},

		Schema: map[string]*schema.Schema{
			"nsx_id":       getNsxIDSchema(),
			"path":         getPathSchema(),
			"display_name": getDisplayNameSchema(),
			"description":  getDescriptionSchema(),
			"revision":     getRevisionSchema(),
			"tag":          getTagsSchema(),
			"site_path": {
				Type:         schema.TypeString,
				Description:  "Path to the site this Host Transport Node belongs to",
				Optional:     true,
				ForceNew:     true,
				Default:      defaultInfraSitePath,
				ValidateFunc: validatePolicyPath(),
			},
			"enforcement_point": {
				Type:        schema.TypeString,
				Description: "ID of the enforcement point this Host Transport Node belongs to",
				Optional:    true,
				ForceNew:    true,
				Computed:    true,
			},
			"discovered_node_id": {
				Type:          schema.TypeString,
				Optional:      true,
				Description:   "Discovered node id to create Host Transport Node",
				ConflictsWith: []string{"node_deployment_info"},
			},
			"node_deployment_info": getFabricHostNodeSchema(),
			// host_switch_spec
			"standard_host_switch":      getStandardHostSwitchSchema(),
			"preconfigured_host_switch": getPreconfiguredHostSwitchSchema(),
		},
	}
}

func getFabricHostNodeSchema() *schema.Schema {
	elemSchema := map[string]*schema.Schema{
		"fqdn": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "Fully qualified domain name of the fabric node",
		},
		"ip_addresses": {
			Type:        schema.TypeList,
			Optional:    true,
			Computed:    true,
			Description: "IP Addresses of the Node, version 4 or 6",
			Elem: &schema.Schema{
				Type:         schema.TypeString,
				ValidateFunc: validateSingleIP(),
			},
		},
	}
	maps.Copy(elemSchema, getSharedHostNodeSchemaAttrs())
	s := schema.Schema{
		Type:     schema.TypeList,
		MaxItems: 1,
		Optional: true,
		Elem: &schema.Resource{
			Schema: elemSchema,
		},
	}
	return &s
}

func resourceNsxtPolicyHostTransportNodeRead(d *schema.ResourceData, m interface{}) error {
	connector := getPolicyConnector(m)
	htnClient := enforcement_points.NewHostTransportNodesClient(connector)

	id, siteID, epID, err := policyIDSiteEPTuple(d, m)
	if err != nil {
		return err
	}

	obj, err := htnClient.Get(siteID, epID, id)
	if err != nil {
		return handleReadError(d, "HostTransportNode", id, err)
	}
	sitePath, err := getSitePathFromChildResourcePath(*obj.ParentPath)
	if err != nil {
		return handleReadError(d, "HostTransportNode", id, err)
	}

	d.Set("site_path", sitePath)
	d.Set("enforcement_point", epID)
	d.Set("display_name", obj.DisplayName)
	d.Set("description", obj.Description)
	setPolicyTagsInSchema(d, obj.Tags)
	d.Set("nsx_id", id)
	d.Set("path", obj.Path)
	d.Set("revision", obj.Revision)

	err = setHostSwitchSpecInSchema(d, obj.HostSwitchSpec)
	if err != nil {
		return err
	}

	fabricHostNode := obj.NodeDeploymentInfo
	elem := make(map[string]interface{})
	elem["fqdn"] = fabricHostNode.Fqdn
	elem["ip_addresses"] = fabricHostNode.IpAddresses

	elem["os_type"] = fabricHostNode.OsType
	elem["os_version"] = fabricHostNode.OsVersion
	elem["windows_install_location"] = fabricHostNode.WindowsInstallLocation

	d.Set("node_deployment_info", []map[string]interface{}{elem})

	return nil
}

func resourceNsxtPolicyHostTransportNodeExists(siteID, epID, tzID string, connector client.Connector) (bool, error) {
	var err error

	// Check site existence first
	siteClient := infra.NewSitesClient(connector)
	_, err = siteClient.Get(siteID)
	if err != nil {
		msg := fmt.Sprintf("failed to read site %s", siteID)
		return false, logAPIError(msg, err)
	}

	// Check (ep, htn) existence. In case of ep not found, NSX returns BAD_REQUEST
	htnClient := enforcement_points.NewHostTransportNodesClient(connector)
	_, err = htnClient.Get(siteID, epID, tzID)
	if err == nil {
		return true, nil
	}

	if isNotFoundError(err) {
		return false, nil
	}

	return false, logAPIError("Error retrieving resource", err)
}

func getFabricHostNodeFromSchema(d *schema.ResourceData) *model2.FabricHostNode {
	for _, ni := range d.Get("node_deployment_info").([]interface{}) {
		nodeInfo := ni.(map[string]interface{})
		ipAddresses := interfaceListToStringList(nodeInfo["ip_addresses"].([]interface{}))

		var hostCredential *model2.HostNodeLoginCredential
		for _, hci := range nodeInfo["host_credential"].([]interface{}) {
			hc := hci.(map[string]interface{})
			password := hc["password"].(string)
			thumbprint := hc["thumbprint"].(string)
			username := hc["username"].(string)
			hostCredential = &model2.HostNodeLoginCredential{
				Password:   &password,
				Thumbprint: &thumbprint,
				Username:   &username,
			}
		}
		osType := nodeInfo["os_type"].(string)
		osVersion := nodeInfo["os_version"].(string)
		windowsInstallLocation := nodeInfo["windows_install_location"].(string)

		fabricHostNode := model2.FabricHostNode{
			IpAddresses:            ipAddresses,
			HostCredential:         hostCredential,
			OsType:                 &osType,
			OsVersion:              &osVersion,
			WindowsInstallLocation: &windowsInstallLocation,
		}
		return &fabricHostNode
	}
	return nil
}

func policyHostTransportNodePatch(siteID, epID, htnID string, d *schema.ResourceData, m interface{}) error {
	connector := getPolicyConnector(m)
	htnClient := enforcement_points.NewHostTransportNodesClient(connector)

	description := d.Get("description").(string)
	displayName := d.Get("display_name").(string)
	tags := getPolicyTagsFromSchema(d)
	discoveredNodeID := d.Get("discovered_node_id").(string)
	nodeDeploymentInfo := getFabricHostNodeFromSchema(d)
	hostSwitchSpec, err := getHostSwitchSpecFromSchema(d)
	revision := int64(d.Get("revision").(int))
	if err != nil {
		return fmt.Errorf("failed to create hostSwitchSpec of HostTransportNode: %v", err)
	}

	obj := model2.HostTransportNode{
		Description:               &description,
		DisplayName:               &displayName,
		Tags:                      tags,
		HostSwitchSpec:            hostSwitchSpec,
		NodeDeploymentInfo:        nodeDeploymentInfo,
		DiscoveredNodeIdForCreate: &discoveredNodeID,
		Revision:                  &revision,
	}

	return htnClient.Patch(siteID, epID, htnID, obj, nil, nil, nil, nil, nil, nil, nil)
}

func resourceNsxtPolicyHostTransportNodeCreate(d *schema.ResourceData, m interface{}) error {
	connector := getPolicyConnector(m)
	id := d.Get("nsx_id").(string)
	if id == "" {
		id = newUUID()
	}
	sitePath := d.Get("site_path").(string)
	siteID := getResourceIDFromResourcePath(sitePath, "sites")
	if siteID == "" {
		return fmt.Errorf("error obtaining Site ID from site path %s", sitePath)
	}
	epID := d.Get("enforcement_point").(string)
	if epID == "" {
		epID = getPolicyEnforcementPoint(m)
	}
	exists, err := resourceNsxtPolicyHostTransportNodeExists(siteID, epID, id, connector)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("resource with ID %s already exists", id)
	}

	// Create the resource using PATCH
	log.Printf("[INFO] Creating HostTransportNode with ID %s under site %s enforcement point %s", id, siteID, epID)
	err = policyHostTransportNodePatch(siteID, epID, id, d, m)
	if err != nil {
		return handleCreateError("HostTransportNode", id, err)
	}

	d.SetId(id)
	d.Set("nsx_id", id)

	return resourceNsxtPolicyHostTransportNodeRead(d, m)
}

func resourceNsxtPolicyHostTransportNodeUpdate(d *schema.ResourceData, m interface{}) error {
	id, siteID, epID, err := policyIDSiteEPTuple(d, m)
	if err != nil {
		return err
	}

	log.Printf("[INFO] Updating HostTransportNode with ID %s", id)
	err = policyHostTransportNodePatch(siteID, epID, id, d, m)
	if err != nil {
		return handleUpdateError("HostTransportNode", id, err)
	}

	return resourceNsxtPolicyHostTransportNodeRead(d, m)
}

func resourceNsxtPolicyHostTransportNodeDelete(d *schema.ResourceData, m interface{}) error {
	connector := getPolicyConnector(m)
	htnClient := enforcement_points.NewHostTransportNodesClient(connector)

	id, siteID, epID, err := policyIDSiteEPTuple(d, m)
	if err != nil {
		return err
	}

	log.Printf("[INFO] Deleting HostTransportNode with ID %s", id)
	err = htnClient.Delete(siteID, epID, id, nil, nil)
	if err != nil {
		return handleDeleteError("HostTransportNode", id, err)
	}

	return nil
}

func resourceNsxtPolicyHostTransportNodeImporter(d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	importID := d.Id()
	rd, err := nsxtPolicyPathResourceImporterHelper(d, m)
	if err != nil {
		return rd, err
	}

	epID, err := getParameterFromPolicyPath("/enforcement-points/", "/host-transport-nodes/", importID)
	if err != nil {
		return nil, err
	}
	d.Set("enforcement_point", epID)
	sitePath, err := getSitePathFromChildResourcePath(importID)
	if err != nil {
		return rd, err
	}
	d.Set("site_path", sitePath)
	return rd, nil
}
