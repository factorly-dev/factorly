package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// DocuSign returns the template for DocuSign e-signatures.
func DocuSign() *Template {
	return &Template{
		Name:        "docusign",
		DisplayName: "DocuSign",
		Description: "Electronic signatures, envelopes, and document management",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get credentials at https://developers.docusign.com",
		VaultKey:    "DOCUSIGN_ACCESS_TOKEN",
		BaseURL:     "https://demo.docusign.net/restapi/v2.1",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_envelopes",
				Description: "List envelopes for an account",
				Method:      "GET",
				Path:        "/accounts/{accountId}/envelopes",
				Parameters: []config.ParamConfig{
					{Name: "accountId", In: "path", Required: true, Description: "Account ID"},
					{Name: "from_date", In: "query", Required: true, Description: "Start date (ISO 8601)"},
					{Name: "status", In: "query", Description: "Filter by status (created, sent, completed, declined, voided)"},
					{Name: "count", In: "query", Description: "Number of results"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_envelope",
				Description: "Get an envelope by ID",
				Method:      "GET",
				Path:        "/accounts/{accountId}/envelopes/{envelopeId}",
				Parameters: []config.ParamConfig{
					{Name: "accountId", In: "path", Required: true, Description: "Account ID"},
					{Name: "envelopeId", In: "path", Required: true, Description: "Envelope ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "send_envelope",
				Description: "Create and send an envelope",
				Method:      "POST",
				Path:        "/accounts/{accountId}/envelopes",
				Parameters: []config.ParamConfig{
					{Name: "accountId", In: "path", Required: true, Description: "Account ID"},
					{Name: "status", In: "body", Required: true, Description: "Envelope status (created, sent)"},
					{Name: "emailSubject", In: "body", Required: true, Description: "Email subject line"},
					{Name: "recipients", In: "body", Required: true, Description: "Recipients object (signers, carbonCopies, etc.)"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "get_document",
				Description: "Get a document from an envelope",
				Method:      "GET",
				Path:        "/accounts/{accountId}/envelopes/{envelopeId}/documents/{documentId}",
				Parameters: []config.ParamConfig{
					{Name: "accountId", In: "path", Required: true, Description: "Account ID"},
					{Name: "envelopeId", In: "path", Required: true, Description: "Envelope ID"},
					{Name: "documentId", In: "path", Required: true, Description: "Document ID"},
				},
				ActionType: "read",
			},
			{
				Name:        "list_recipients",
				Description: "List recipients of an envelope",
				Method:      "GET",
				Path:        "/accounts/{accountId}/envelopes/{envelopeId}/recipients",
				Parameters: []config.ParamConfig{
					{Name: "accountId", In: "path", Required: true, Description: "Account ID"},
					{Name: "envelopeId", In: "path", Required: true, Description: "Envelope ID"},
				},
				ActionType: "read",
			},
		},
	}
}
