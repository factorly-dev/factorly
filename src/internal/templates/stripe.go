package templates

import _ "embed"

//go:embed yaml/stripe.yaml
var stripeYAML string

// Stripe returns the template for Stripe.
func Stripe() *Template {
	return &Template{
		Name:        "stripe",
		DisplayName: "Stripe",
		Description: "Payment processing, subscriptions, and billing",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your secret key at https://dashboard.stripe.com/apikeys",
		VaultKey:    "STRIPE_SECRET_KEY",
		YAML:        stripeYAML,
	}
}
