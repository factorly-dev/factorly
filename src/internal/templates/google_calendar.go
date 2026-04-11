package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// GoogleCalendar returns the template for Google Calendar event management.
func GoogleCalendar() *Template {
	return &Template{
		Name:        "google-calendar",
		DisplayName: "Google Calendar",
		Description: "Manage calendars, events, and scheduling",
		Category:    "business",
		AuthType:    "bearer",
		AuthGuide:   "Create credentials at https://console.cloud.google.com/apis/credentials",
		VaultKey:    "GOOGLE_API_TOKEN",
		BaseURL:     "https://www.googleapis.com/calendar/v3",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Tools: []ToolDef{
			{
				Name:        "list_events",
				Description: "List events on a calendar",
				Method:      "GET",
				Path:        "/calendars/{{calendarId}}/events",
				Parameters: []config.ParamConfig{
					{Name: "calendarId", In: "path", Required: true, Description: "Calendar ID (use 'primary' for the main calendar)"},
					{Name: "timeMin", In: "query", Description: "Lower bound for event start time (RFC3339 timestamp)"},
					{Name: "timeMax", In: "query", Description: "Upper bound for event start time (RFC3339 timestamp)"},
					{Name: "maxResults", In: "query", Description: "Maximum number of events to return"},
					{Name: "singleEvents", In: "query", Description: "Whether to expand recurring events (true/false)"},
					{Name: "orderBy", In: "query", Description: "Sort order (startTime or updated)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "create_event",
				Description: "Create a new calendar event",
				Method:      "POST",
				Path:        "/calendars/{{calendarId}}/events",
				Parameters: []config.ParamConfig{
					{Name: "calendarId", In: "path", Required: true, Description: "Calendar ID (use 'primary' for the main calendar)"},
					{Name: "summary", In: "body", Required: true, Description: "Event title"},
					{Name: "description", In: "body", Description: "Event description"},
					{Name: "start", In: "body", Required: true, Description: "Start time object with dateTime and timeZone"},
					{Name: "end", In: "body", Required: true, Description: "End time object with dateTime and timeZone"},
					{Name: "attendees", In: "body", Description: "Array of attendees with email addresses"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "get_event",
				Description: "Get a specific calendar event",
				Method:      "GET",
				Path:        "/calendars/{{calendarId}}/events/{{eventId}}",
				Parameters: []config.ParamConfig{
					{Name: "calendarId", In: "path", Required: true, Description: "Calendar ID (use 'primary' for the main calendar)"},
					{Name: "eventId", In: "path", Required: true, Description: "Event ID"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "update_event",
				Description: "Update an existing calendar event",
				Method:      "PUT",
				Path:        "/calendars/{{calendarId}}/events/{{eventId}}",
				Parameters: []config.ParamConfig{
					{Name: "calendarId", In: "path", Required: true, Description: "Calendar ID (use 'primary' for the main calendar)"},
					{Name: "eventId", In: "path", Required: true, Description: "Event ID"},
					{Name: "summary", In: "body", Description: "Event title"},
					{Name: "description", In: "body", Description: "Event description"},
					{Name: "start", In: "body", Description: "Start time object with dateTime and timeZone"},
					{Name: "end", In: "body", Description: "End time object with dateTime and timeZone"},
				},
				ActionType: "write",
			},
			{
				Name:        "delete_event",
				Description: "Delete a calendar event",
				Method:      "DELETE",
				Path:        "/calendars/{{calendarId}}/events/{{eventId}}",
				Parameters: []config.ParamConfig{
					{Name: "calendarId", In: "path", Required: true, Description: "Calendar ID (use 'primary' for the main calendar)"},
					{Name: "eventId", In: "path", Required: true, Description: "Event ID"},
				},
				ActionType: "delete",
			},
			{
				Name:        "list_calendars",
				Description: "List all calendars for the authenticated user",
				Method:      "GET",
				Path:        "/users/me/calendarList",
				Parameters: []config.ParamConfig{
					{Name: "maxResults", In: "query", Description: "Maximum number of calendars to return"},
				},
				ActionType: "read",
			},
		},
	}
}
