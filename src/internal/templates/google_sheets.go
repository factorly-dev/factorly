package templates

import "github.com/factorly-dev/factorly-cli/internal/config"

// GoogleSheets returns the template for Google Sheets spreadsheet access.
func GoogleSheets() *Template {
	return &Template{
		Name:        "google-sheets",
		DisplayName: "Google Sheets",
		Description: "Read and write Google Sheets spreadsheets",
		Category:    "business",
		AuthType:    "oauth",
		AuthGuide:   "Create OAuth credentials at https://console.cloud.google.com/apis/credentials",
		VaultKey:    "", // OAuth uses provider-based auth, not a single vault key
		OAuthConfig: &OAuthConfig{
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
			Scopes:   []string{"https://www.googleapis.com/auth/spreadsheets"},
		},
		BaseURL:     "https://sheets.googleapis.com",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Tools: []ToolDef{
			{
				Name:        "get_spreadsheet",
				Description: "Get spreadsheet metadata and sheets",
				Method:      "GET",
				Path:        "/v4/spreadsheets/{{spreadsheetId}}",
				Parameters: []config.ParamConfig{
					{Name: "spreadsheetId", In: "path", Required: true, Description: "The spreadsheet ID"},
					{Name: "includeGridData", In: "query", Description: "True to include grid data"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "get_values",
				Description: "Get values from a spreadsheet range",
				Method:      "GET",
				Path:        "/v4/spreadsheets/{{spreadsheetId}}/values/{{range}}",
				Parameters: []config.ParamConfig{
					{Name: "spreadsheetId", In: "path", Required: true, Description: "The spreadsheet ID"},
					{Name: "range", In: "path", Required: true, Description: "A1 notation range (e.g. Sheet1!A1:D5)"},
					{Name: "majorDimension", In: "query", Description: "Major dimension (ROWS or COLUMNS)"},
					{Name: "valueRenderOption", In: "query", Description: "How values are rendered (FORMATTED_VALUE, UNFORMATTED_VALUE, FORMULA)"},
				},
				ActionType: "read",
				Essential:  true,
			},
			{
				Name:        "update_values",
				Description: "Update values in a spreadsheet range",
				Method:      "PUT",
				Path:        "/v4/spreadsheets/{{spreadsheetId}}/values/{{range}}",
				Parameters: []config.ParamConfig{
					{Name: "spreadsheetId", In: "path", Required: true, Description: "The spreadsheet ID"},
					{Name: "range", In: "path", Required: true, Description: "A1 notation range (e.g. Sheet1!A1:D5)"},
					{Name: "valueInputOption", In: "query", Required: true, Description: "How input is interpreted (RAW or USER_ENTERED)"},
					{Name: "values", In: "body", Required: true, Description: "2D array of values to write"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "append_values",
				Description: "Append values to a spreadsheet range",
				Method:      "POST",
				Path:        "/v4/spreadsheets/{{spreadsheetId}}/values/{{range}}:append",
				Parameters: []config.ParamConfig{
					{Name: "spreadsheetId", In: "path", Required: true, Description: "The spreadsheet ID"},
					{Name: "range", In: "path", Required: true, Description: "A1 notation range to search for a table"},
					{Name: "valueInputOption", In: "query", Required: true, Description: "How input is interpreted (RAW or USER_ENTERED)"},
					{Name: "insertDataOption", In: "query", Description: "How input data is inserted (OVERWRITE or INSERT_ROWS)"},
					{Name: "values", In: "body", Required: true, Description: "2D array of values to append"},
				},
				ActionType: "write",
				Essential:  true,
			},
			{
				Name:        "create_spreadsheet",
				Description: "Create a new spreadsheet",
				Method:      "POST",
				Path:        "/v4/spreadsheets",
				Parameters: []config.ParamConfig{
					{Name: "title", In: "body", Required: true, Description: "Title for the new spreadsheet"},
				},
				ActionType: "write",
			},
			{
				Name:        "clear_values",
				Description: "Clear values from a spreadsheet range",
				Method:      "POST",
				Path:        "/v4/spreadsheets/{{spreadsheetId}}/values/{{range}}:clear",
				Parameters: []config.ParamConfig{
					{Name: "spreadsheetId", In: "path", Required: true, Description: "The spreadsheet ID"},
					{Name: "range", In: "path", Required: true, Description: "A1 notation range to clear"},
				},
				ActionType: "write",
			},
		},
	}
}
