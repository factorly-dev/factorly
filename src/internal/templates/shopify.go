package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Shopify returns the template for Shopify e-commerce.
func Shopify() *Template {
	return &Template{
		Name:        "shopify",
		DisplayName: "Shopify",
		Description: "E-commerce, orders, products, and inventory management",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Create a custom app at https://YOUR_STORE.myshopify.com/admin/settings/apps",
		VaultKey:    "SHOPIFY_ACCESS_TOKEN",
		BaseURL:     "https://YOUR_STORE.myshopify.com/admin/api/2024-01",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_orders",
				Description: "List orders",
				Method:      "GET",
				Path:        "/orders.json",
				Parameters: []config.ParamConfig{
					{Name: "limit", In: "query", Description: "Number of results (1-250)"},
					{Name: "status", In: "query", Description: "Filter by status (open, closed, cancelled, any)"},
					{Name: "since_id", In: "query", Description: "Show orders after this ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_products",
				Description: "List products",
				Method:      "GET",
				Path:        "/products.json",
				Parameters: []config.ParamConfig{
					{Name: "limit", In: "query", Description: "Number of results (1-250)"},
					{Name: "since_id", In: "query", Description: "Show products after this ID"},
					{Name: "title", In: "query", Description: "Filter by title"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_order",
				Description: "Get an order by ID",
				Method:      "GET",
				Path:        "/orders/{order_id}.json",
				Parameters: []config.ParamConfig{
					{Name: "order_id", In: "path", Required: true, Description: "Order ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_customers",
				Description: "List customers",
				Method:      "GET",
				Path:        "/customers.json",
				Parameters: []config.ParamConfig{
					{Name: "limit", In: "query", Description: "Number of results (1-250)"},
					{Name: "since_id", In: "query", Description: "Show customers after this ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_product",
				Description: "Create a new product",
				Method:      "POST",
				Path:        "/products.json",
				Parameters: []config.ParamConfig{
					{Name: "product", In: "body", Required: true, Description: "Product object with title, body_html, vendor, etc."},
				},
				ActionType: "write",
			},
			{
				Name:        "update_inventory",
				Description: "Set inventory level for an item",
				Method:      "POST",
				Path:        "/inventory_levels/set.json",
				Parameters: []config.ParamConfig{
					{Name: "inventory_item_id", In: "body", Required: true, Description: "Inventory item ID"},
					{Name: "location_id", In: "body", Required: true, Description: "Location ID"},
					{Name: "available", In: "body", Required: true, Description: "Available quantity"},
				},
				ActionType: "write",
			},
		},
	}
}
