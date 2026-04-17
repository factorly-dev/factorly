// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/shopify.yaml
var shopifyYAML string

// Shopify returns the template for Shopify.
func Shopify() *Template {
	return &Template{
		Name:        "shopify",
		DisplayName: "Shopify",
		Description: "E-commerce, orders, products, and inventory management",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Create a custom app at https://YOUR_STORE.myshopify.com/admin/settings/apps",
		VaultKey:    "SHOPIFY_ACCESS_TOKEN",
		YAML:        shopifyYAML,
	}
}
