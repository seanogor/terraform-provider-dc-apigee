package provider

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// resourceExample is an example resource definition
func resourceExample() *schema.Resource {
	return &schema.Resource{
		Create: resourceExampleCreate,
		Read:   resourceExampleRead,
		Update: resourceExampleUpdate,
		Delete: resourceExampleDelete,

		Schema: map[string]*schema.Schema{
			"example_attribute": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
	}
}

func resourceExampleCreate(d *schema.ResourceData, m interface{}) error {
	// Implement create logic
	d.SetId("example_id")
	return resourceExampleRead(d, m)
}

func resourceExampleRead(d *schema.ResourceData, m interface{}) error {
	// Implement read logic
	return nil
}

func resourceExampleUpdate(d *schema.ResourceData, m interface{}) error {
	// Implement update logic
	return resourceExampleRead(d, m)
}

func resourceExampleDelete(d *schema.ResourceData, m interface{}) error {
	// Implement delete logic
	d.SetId("")
	return nil
}
