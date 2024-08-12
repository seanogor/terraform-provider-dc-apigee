package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

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
		return nil, diag.FromErr(err)
	}

	client := config.Client(ctx)
	token, err := config.TokenSource(ctx).Token()
	if err != nil {
		return nil, diag.FromErr(err)
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
	dcNamesInterface = d.Get("dc_names")
	if dcNamesInterface == nil {
		return nil, diag.Errorf("data collector names (dc_names) are not set")
	}
	dcNames, ok = dcNamesInterface.([]interface{})
	if !ok {
		return nil, diag.Errorf("data collector names (dc_names) are not a list of strings")
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
		DeleteContext: resourceDataCollectorDeleteFunc,

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
	diags := resourceDataCollectorCreateFunc(ctx, d, m)
	if diags.HasError() {
		return diag.Errorf("failed to create resource: %v", diags)
	}
	return nil
}

func getGoogleAccessToken(ctx context.Context) (string, error) {
	creds, err := google.FindDefaultCredentials(ctx)
	if err != nil {
		return "", err
	}
	tokenSource := creds.TokenSource
	token, err := tokenSource.Token()
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func resourceDataCollectorCreateFunc(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	name := d.Get("name").(string)
	description := d.Get("description").(string)
	dataType := d.Get("type").(string)

	client := m.(*http.Client)
	accessToken, err := getGoogleAccessToken(ctx)
	if err != nil {
		return diag.FromErr(err)
	}

	payload := map[string]string{
		"name":        name,
		"description": description,
		"type":        dataType,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return diag.FromErr(err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("https://apigee.googleapis.com/v1/organizations/%s/datacollectors", "ORG"), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return diag.FromErr(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := client.Do(req)
	if err != nil {
		return diag.FromErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return diag.Errorf("failed to create data collector: %s", resp.Status)
	}

	d.SetId(name)
	return diags
}

func resourceDataCollectorReadFunc(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	client := m.(*http.Client)
	name := d.Id()
	accessToken, err := getGoogleAccessToken(ctx)
	if err != nil {
		return diag.FromErr(err)
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://apigee.googleapis.com/v1/organizations/%s/datacollectors/%s", "ORG", name), nil)
	if err != nil {
		return diag.FromErr(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := client.Do(req)
	if err != nil {
		return diag.FromErr(err)
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
		return diag.FromErr(err)
	}

	if err := d.Set("name", data["name"]); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("description", data["description"]); err != nil {
		return diag.FromErr(err)
	}
	if err := d.Set("type", data["type"]); err != nil {
		return diag.FromErr(err)
	}

	return diags
}

func resourceDataCollectorUpdateFunc(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	name := d.Get("name").(string)
	description := d.Get("description").(string)
	dataType := d.Get("type").(string)

	client := m.(*http.Client)
	accessToken, err := getGoogleAccessToken(ctx)
	if err != nil {
		return diag.FromErr(err)
	}

	payload := map[string]string{
		"description": description,
		"type":        dataType,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return diag.FromErr(err)
	}

	req, err := http.NewRequest("PUT", fmt.Sprintf("https://apigee.googleapis.com/v1/organizations/%s/datacollectors/%s", "kyc-apigee-nprd", name), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return diag.FromErr(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := client.Do(req)
	if err != nil {
		return diag.FromErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return diag.Errorf("failed to update data collector: %s", resp.Status)
	}

	return diags
}

func resourceDataCollectorDeleteFunc(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var diags diag.Diagnostics

	client := m.(*http.Client)
	name := d.Id()
	accessToken, err := getGoogleAccessToken(ctx)
	if err != nil {
		return diag.FromErr(err)
	}

	req, err := http.NewRequest("DELETE", fmt.Sprintf("https://apigee.googleapis.com/v1/organizations/%s/datacollectors/%s", "kyc-apigee-nprd", name), nil)
	if err != nil {
		return diag.FromErr(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

	resp, err := client.Do(req)
	if err != nil {
		return diag.FromErr(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return diag.Errorf("failed to delete data collector: %s", resp.Status)
	}

	d.SetId("")
	return diags
}
