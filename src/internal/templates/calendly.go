package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// Calendly returns the template for Calendly scheduling.
func Calendly() *Template {
	return &Template{
		Name:        "calendly",
		DisplayName: "Calendly",
		Description: "Scheduling, events, and calendar management",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Get your token at https://calendly.com/integrations/api_webhooks",
		VaultKey:    "CALENDLY_API_KEY",
		BaseURL:     "https://api.calendly.com",
		Headers:     nil,
		Tools: []ToolDef{
			{
				Name:        "list_events",
				Description: "List scheduled events",
				Method:      "GET",
				Path:        "/scheduled_events",
				Parameters: []config.ParamConfig{
					{Name: "user", In: "query", Required: true, Description: "User URI"},
					{Name: "count", In: "query", Description: "Number of results (1-100)"},
					{Name: "status", In: "query", Description: "Filter by status (active, canceled)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_event",
				Description: "Get a scheduled event by UUID",
				Method:      "GET",
				Path:        "/scheduled_events/{event_uuid}",
				Parameters: []config.ParamConfig{
					{Name: "event_uuid", In: "path", Required: true, Description: "Event UUID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_event_types",
				Description: "List available event types",
				Method:      "GET",
				Path:        "/event_types",
				Parameters: []config.ParamConfig{
					{Name: "user", In: "query", Required: true, Description: "User URI"},
					{Name: "count", In: "query", Description: "Number of results"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "list_invitees",
				Description: "List invitees for a scheduled event",
				Method:      "GET",
				Path:        "/scheduled_events/{event_uuid}/invitees",
				Parameters: []config.ParamConfig{
					{Name: "event_uuid", In: "path", Required: true, Description: "Event UUID"},
					{Name: "count", In: "query", Description: "Number of results"},
				},
				ActionType: "read",
			},
			{
				Name:        "get_user",
				Description: "Get current user info",
				Method:      "GET",
				Path:        "/users/me",
				Parameters:  nil,
				ActionType:  "read",
			},
		},
	}
}
