package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// SendGrid returns the template for SendGrid email delivery.
func SendGrid() *Template {
	return &Template{
		Name:        "sendgrid",
		DisplayName: "SendGrid",
		Description: "Email delivery, contacts, and marketing campaigns",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Create an API key at https://app.sendgrid.com/settings/api_keys",
		VaultKey:    "SENDGRID_API_KEY",
		BaseURL:     "https://api.sendgrid.com/v3",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "send_email",
				Description: "Send an email",
				Method:      "POST",
				Path:        "/mail/send",
				Parameters: []config.ParamConfig{
					{Name: "personalizations", In: "body", Required: true, Description: "Array of recipients with to, subject, etc."},
					{Name: "from", In: "body", Required: true, Description: "Sender email object (email, name)"},
					{Name: "content", In: "body", Required: true, Description: "Array of content objects (type, value)"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "list_contacts",
				Description: "List marketing contacts",
				Method:      "GET",
				Path:        "/marketing/contacts",
				Parameters:  nil,
				ActionType:  "read",
				Essential:   true,
			},
			{
				Name:        "list_templates",
				Description: "List email templates",
				Method:      "GET",
				Path:        "/templates",
				Parameters: []config.ParamConfig{
					{Name: "generations", In: "query", Description: "Template generation (legacy, dynamic)"},
					{Name: "page_size", In: "query", Description: "Number of results per page"},
				},
				ActionType: "read",
			},
			{
				Name:        "get_stats",
				Description: "Get email sending statistics",
				Method:      "GET",
				Path:        "/stats",
				Parameters: []config.ParamConfig{
					{Name: "start_date", In: "query", Required: true, Description: "Start date (YYYY-MM-DD)"},
					{Name: "end_date", In: "query", Description: "End date (YYYY-MM-DD)"},
				},
				ActionType: "read",
			},
			{
				Name:        "add_contact",
				Description: "Add or update a marketing contact",
				Method:      "PUT",
				Path:        "/marketing/contacts",
				Parameters: []config.ParamConfig{
					{Name: "contacts", In: "body", Required: true, Description: "Array of contact objects (email, first_name, last_name, etc.)"},
				},
				ActionType: "write",
			},
		},
	}
}
