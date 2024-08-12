package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"golang.org/x/oauth2/google"
)

// Provider returns a schema.Provider for the Terraform provider
func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"dc_names": {
				Type:        schema.TypeList,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("DC_NAMES", nil),
				Description: "The data collector names",
			},
			"org_name": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("ORG_NAME", nil),
				Description: "The organization name",
			},
			"google_credentials": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("GOOGLE_CREDENTIALS", nil),
				Description: "The Google credentials JSON",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"dc_collector": createResourceDataCollector(),
		},
		ConfigureContextFunc: providerConfigure,
	}
}

func providerConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	var diags diag.Diagnostics

	credentials := d.Get("google_credentials").(string)
	config, err := google.JWTConfigFromJSON([]byte(credentials), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, diag.Errorf("failed to operate on data collector: %v", err)
	}

	client := config.Client(ctx)
	token, err := config.TokenSource(ctx).Token()
	if err != nil {
		return nil, diag.Errorf("failed to operate on data collector: %v", err)
	}

	dcNamesInterface := d.Get("dc_names")
	if dcNamesInterface == nil {
		return nil, diag.Errorf("data collector names (dc_names) are not set")
	}
	dcNames, ok := dcNamesInterface.([]interface{})
	if !ok {
		return nil, diag.Errorf("data collector names (dc_names) are not a list of strings")
	}

	orgNameInterface := d.Get("org_name")
	if orgNameInterface == nil {
		return nil, diag.Errorf("organization name (org_name) is not set")
	}
	orgName, ok := orgNameInterface.(string)
	if !ok {
		return nil, diag.Errorf("organization name (org_name) is not a string")
	}

	dcNamesList := make([]string, len(dcNames))
	for i, dcName := range dcNames {
		dcNamesList[i], ok = dcName.(string)
		if !ok {
			return nil, diag.Errorf("data collector name (dc_names) at index %d is not a string", i)
		}
	}

	return map[string]interface{}{
		"client":   client,
		"token":    token.AccessToken,
		"dc_names": dcNamesList,
		"org_name": orgName,
	}, diags
}

func createResourceDataCollector() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceDataCollectorCreateWrapper,
		ReadContext:   resourceDataCollectorReadFunc,
		UpdateContext: resourceDataCollectorUpdateFunc,
		DeleteContext: resourceDataCollectorDelete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceDataCollectorImport,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"description": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"type": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func resourceDataCollectorCreateWrapper(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	config := m.(map[string]interface{})
	client := config["client"].(*http.Client)
	token := config["token"].(string)

	return resourceDataCollectorCreateFunc(ctx, d, client, token)
}

func resourceDataCollectorCreateFunc(ctx context.Context, d *schema.ResourceData, client *http.Client, token string) diag.Diagnostics {
	var diags diag.Diagnostics

	name := d.Get("name").(string)
	description := d.Get("description").(string)
	dataType := d.Get("type").(string)

	payload := map[string]string{
		"name":        name,
		"description": description,
		"type":        dataType,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("https://apigee.googleapis.com/v1/organizations/%s/datacollectors", "ORG"), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return diag.Errorf("failed to create data collector: %s", resp.Status)
	}

	d.SetId(name)
	return diags
}

func resourceDataCollectorReadFunc(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	config := m.(map[string]interface{})
	client := config["client"].(*http.Client)
	token := config["token"].(string)
	org := config["org"].(string)

	return resourceDataCollectorRead(ctx, d, client, token, org)
}

func resourceDataCollectorRead(ctx context.Context, d *schema.ResourceData, client *http.Client, token string, org string) diag.Diagnostics {
	var diags diag.Diagnostics

	name := d.Id()

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://apigee.googleapis.com/v1/organizations/%s/datacollectors/%s", org, name), nil)
	if err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			d.SetId("")
			return diags
		}
		return diag.Errorf("failed to read data collector: %s", resp.Status)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}

	if err := d.Set("name", data["name"]); err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}
	if err := d.Set("description", data["description"]); err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}
	if err := d.Set("type", data["type"]); err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}

	return diags
}

func resourceDataCollectorUpdateFunc(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	config := m.(map[string]interface{})
	client := config["client"].(*http.Client)
	token := config["token"].(string)

	return resourceDataCollectorUpdate(ctx, d, client, token)
}

func resourceDataCollectorUpdate(ctx context.Context, d *schema.ResourceData, client *http.Client, token string) diag.Diagnostics {
	var diags diag.Diagnostics

	name := d.Get("name").(string)
	description := d.Get("description").(string)
	dataType := d.Get("type").(string)

	payload := map[string]string{
		"description": description,
		"type":        dataType,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", fmt.Sprintf("https://apigee.googleapis.com/v1/organizations/%s/datacollectors/%s", "ORG", name), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return diag.Errorf("failed to update data collector: %s", resp.Status)
	}

	return diags
}

func resourceDataCollectorDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	config := m.(map[string]interface{})
	client := config["client"].(*http.Client)
	token := config["token"].(string)

	return resourceDataCollectorDeleteFunc(ctx, d, client, token)
}

func resourceDataCollectorDeleteFunc(ctx context.Context, d *schema.ResourceData, client *http.Client, token string) diag.Diagnostics {
	var diags diag.Diagnostics

	name := d.Id()

	req, err := http.NewRequestWithContext(ctx, "DELETE", fmt.Sprintf("https://apigee.googleapis.com/v1/organizations/%s/datacollectors/%s", "ORG", name), nil)
	if err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return diag.Errorf("failed to operate on data collector: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return diag.Errorf("failed to delete data collector: %s", resp.Status)
	}

	d.SetId("")
	return diags
}

func resourceDataCollectorImport(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, error) {
	config := m.(map[string]interface{})
	client := config["client"].(*http.Client)
	token := config["token"].(string)
	org := config["org"].(string)

	resources, diags := resourceDataCollectorImportFunc(ctx, d, map[string]interface{}{"client": client, "token": token, "org": org})
	if diags != nil {
		return resources, fmt.Errorf("failed to import data collector: %v", diags)
	}
	return resources, nil
}

func resourceDataCollectorImportFunc(ctx context.Context, d *schema.ResourceData, m interface{}) ([]*schema.ResourceData, diag.Diagnostics) {
	config := m.(map[string]interface{})
	client := config["client"].(*http.Client)
	token := config["token"].(string)
	org := config["org"].(string)

	id := d.Id()
	parts := strings.Split(id, "/")
	if len(parts) != 2 {
		return nil, diag.Errorf("unexpected format of ID (%s), expected project_id/name", id)
	}

	projectID := parts[0]
	name := parts[1]

	d.Set("project_id", projectID)
	d.Set("name", name)

	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://apigee.googleapis.com/v1/organizations/%s/datacollectors/%s", org, name), nil)
	if err != nil {
		return nil, diag.Errorf("failed to operate on data collector: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return nil, diag.Errorf("failed to operate on data collector: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			d.SetId("")
			return []*schema.ResourceData{d}, nil
		}
		return nil, diag.Errorf("failed to read data collector: %s", resp.Status)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, diag.Errorf("failed to import data collector: %v", err)
	}

	if err := d.Set("name", data["name"]); err != nil {
		return nil, diag.Errorf("failed to import data collector: %v", err)
	}
	if err := d.Set("description", data["description"]); err != nil {
		return nil, diag.Errorf("failed to import data collector: %v", err)
	}
	if err := d.Set("type", data["type"]); err != nil {
		return nil, diag.Errorf("failed to import data collector: %v", err)
	}

	return []*schema.ResourceData{d}, nil
}
