// Copyright 2026 Jordan Sherer <hi@jordansherer.com>
// SPDX-License-Identifier: gpl

package templates

import _ "embed"

//go:embed yaml/google-calendar.yaml
var googleCalendarYAML string

// GoogleCalendar returns the template for Google Calendar.
func GoogleCalendar() *Template {
	return &Template{
		Name:        "google-calendar",
		DisplayName: "Google Calendar",
		Description: "Manage calendars, events, and scheduling",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Create OAuth credentials at https://console.cloud.google.com/apis/credentials",
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes:   []string{"https://www.googleapis.com/auth/calendar"},
		},
		YAML: googleCalendarYAML,
	}
}
