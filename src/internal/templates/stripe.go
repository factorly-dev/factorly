package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Stripe returns the template for Stripe payment processing.
func Stripe() *Template {
	return &Template{
		Name:        "stripe",
		DisplayName: "Stripe",
		Description: "Payment processing, subscriptions, and billing",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your secret key at https://dashboard.stripe.com/apikeys",
		VaultKey:    "STRIPE_SECRET_KEY",
		BaseURL:     "https://api.stripe.com",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_customers",
				Description: "List customers",
				Method:      "GET",
				Path:        "/v1/customers",
				Parameters: []config.ParamConfig{
					{Name: "limit", In: "query", Description: "Number of results (1-100)"},
					{Name: "email", In: "query", Description: "Filter by email address"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_customer",
				Description: "Create a new customer",
				Method:      "POST",
				Path:        "/v1/customers",
				Parameters: []config.ParamConfig{
					{Name: "email", In: "body", Required: true, Description: "Customer email"},
					{Name: "name", In: "body", Description: "Customer name"},
					{Name: "description", In: "body", Description: "Customer description"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "list_charges",
				Description: "List charges",
				Method:      "GET",
				Path:        "/v1/charges",
				Parameters: []config.ParamConfig{
					{Name: "limit", In: "query", Description: "Number of results (1-100)"},
					{Name: "customer", In: "query", Description: "Filter by customer ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_invoices",
				Description: "List invoices",
				Method:      "GET",
				Path:        "/v1/invoices",
				Parameters: []config.ParamConfig{
					{Name: "limit", In: "query", Description: "Number of results (1-100)"},
					{Name: "customer", In: "query", Description: "Filter by customer ID"},
					{Name: "status", In: "query", Description: "Filter by status (draft, open, paid, void, uncollectible)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_subscriptions",
				Description: "List subscriptions",
				Method:      "GET",
				Path:        "/v1/subscriptions",
				Parameters: []config.ParamConfig{
					{Name: "limit", In: "query", Description: "Number of results (1-100)"},
					{Name: "customer", In: "query", Description: "Filter by customer ID"},
					{Name: "status", In: "query", Description: "Filter by status (active, past_due, canceled, etc.)"},
				},
				ActionType: "read",
			},
			{
				Name:        "create_invoice",
				Description: "Create an invoice",
				Method:      "POST",
				Path:        "/v1/invoices",
				Parameters: []config.ParamConfig{
					{Name: "customer", In: "body", Required: true, Description: "Customer ID"},
					{Name: "auto_advance", In: "body", Description: "Auto-finalize the invoice (true/false)"},
				},
				ActionType: "write",
			},
			{
				Name:        "refund_charge",
				Description: "Refund a charge",
				Method:      "POST",
				Path:        "/v1/refunds",
				Parameters: []config.ParamConfig{
					{Name: "charge", In: "body", Required: true, Description: "Charge ID to refund"},
					{Name: "amount", In: "body", Description: "Amount in cents (partial refund)"},
				},
				ActionType: "write",
			},
			{
				Name:        "get_balance",
				Description: "Get account balance",
				Method:      "GET",
				Path:        "/v1/balance",
				Parameters:  nil,
				ActionType:  "read",
				Essential:   true,
			},
		},
	}
}
