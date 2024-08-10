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
			"dc_name": {
				Type:     schema.TypeString,
				Required: true,
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

	// Example configuration logic
	config := map[string]interface{}{
		"org_name":     d.Get("org_name").(string),
		"access_token": d.Get("access_token").(string),
	}

	return config, diags
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
