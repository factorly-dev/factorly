package openapi

import (
	"os"
	"path/filepath"
	"testing"
)

const petstoreSpec = `
openapi: "3.0.0"
info:
  title: Pet Store
  version: "1.0.0"
servers:
  - url: https://petstore.example.com/v1
paths:
  /pets:
    get:
      operationId: listPets
      summary: List all pets
      parameters:
        - name: limit
          in: query
          description: How many items to return
          required: false
          schema:
            type: integer
    post:
      operationId: createPet
      summary: Create a pet
      requestBody:
        required: true
        description: Pet object to create
        content:
          application/json:
            schema:
              type: object
  /pets/{petId}:
    get:
      operationId: showPetById
      summary: Info for a specific pet
      parameters:
        - name: petId
          in: path
          required: true
          description: The id of the pet to retrieve
          schema:
            type: string
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
`

func writeSpec(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGeneratePetstore(t *testing.T) {
	path := writeSpec(t, petstoreSpec)
	tools, err := Generate(path, GenerateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}

	// Check listPets
	list, ok := tools["pet_store.listPets"]
	if !ok {
		t.Fatal("expected tool pet_store.listPets")
	}
	if list.Type != "rest" {
		t.Errorf("expected type rest, got %s", list.Type)
	}
	if list.Method != "GET" {
		t.Errorf("expected method GET, got %s", list.Method)
	}
	if list.Path != "/pets" {
		t.Errorf("expected path /pets, got %s", list.Path)
	}
	if list.BaseURL != "https://petstore.example.com/v1" {
		t.Errorf("expected base_url https://petstore.example.com/v1, got %s", list.BaseURL)
	}
	if list.Description != "List all pets" {
		t.Errorf("expected description 'List all pets', got %q", list.Description)
	}
	if len(list.Parameters) != 1 {
		t.Fatalf("expected 1 param for listPets, got %d", len(list.Parameters))
	}
	if list.Parameters[0].Name != "limit" {
		t.Errorf("expected param name 'limit', got %q", list.Parameters[0].Name)
	}
	if list.Parameters[0].In != "query" {
		t.Errorf("expected param in 'query', got %q", list.Parameters[0].In)
	}

	// Check createPet has body param
	create, ok := tools["pet_store.createPet"]
	if !ok {
		t.Fatal("expected tool pet_store.createPet")
	}
	if create.Method != "POST" {
		t.Errorf("expected method POST, got %s", create.Method)
	}
	if len(create.Parameters) != 1 {
		t.Fatalf("expected 1 param (body) for createPet, got %d", len(create.Parameters))
	}
	if create.Parameters[0].In != "body" {
		t.Errorf("expected body param, got in=%q", create.Parameters[0].In)
	}

	// Check showPetById
	show, ok := tools["pet_store.showPetById"]
	if !ok {
		t.Fatal("expected tool pet_store.showPetById")
	}
	if show.Path != "/pets/{{petId}}" {
		t.Errorf("expected path /pets/{{petId}}, got %s", show.Path)
	}
	if len(show.Parameters) != 1 {
		t.Fatalf("expected 1 param for showPetById, got %d", len(show.Parameters))
	}
	if !show.Parameters[0].Required {
		t.Error("expected path param petId to be required")
	}
}

func TestGenerateWithPrefix(t *testing.T) {
	path := writeSpec(t, petstoreSpec)
	tools, err := Generate(path, GenerateOpts{Prefix: "myapi"})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := tools["myapi.listPets"]; !ok {
		t.Error("expected tool myapi.listPets with custom prefix")
	}
}

func TestGenerateAuth(t *testing.T) {
	path := writeSpec(t, petstoreSpec)
	tools, err := Generate(path, GenerateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	for name, tool := range tools {
		if tool.Auth == nil {
			t.Errorf("tool %s: expected auth config", name)
			continue
		}
		if tool.Auth.Type != "bearer" {
			t.Errorf("tool %s: expected bearer auth, got %s", name, tool.Auth.Type)
		}
		if tool.Auth.Token != "{{env:PET_STORE_TOKEN}}" {
			t.Errorf("tool %s: expected {{env:PET_STORE_TOKEN}}, got %s", name, tool.Auth.Token)
		}
	}
}

func TestGenerateAPIKeyAuth(t *testing.T) {
	spec := `
openapi: "3.0.0"
info:
  title: My API
servers:
  - url: https://api.example.com
paths:
  /data:
    get:
      operationId: getData
      summary: Get data
components:
  securitySchemes:
    apiKey:
      type: apiKey
      name: X-Custom-Key
      in: header
`
	path := writeSpec(t, spec)
	tools, err := Generate(path, GenerateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	tool := tools["my_api.getData"]
	if tool.Auth == nil {
		t.Fatal("expected auth config")
	}
	if tool.Auth.Type != "header" {
		t.Errorf("expected header auth, got %s", tool.Auth.Type)
	}
	if tool.Auth.Header != "X-Custom-Key" {
		t.Errorf("expected header X-Custom-Key, got %s", tool.Auth.Header)
	}
}

func TestGenerateNoOperationID(t *testing.T) {
	spec := `
openapi: "3.0.0"
info:
  title: Minimal
servers:
  - url: https://api.example.com
paths:
  /users/{id}:
    get:
      summary: Get a user
`
	path := writeSpec(t, spec)
	tools, err := Generate(path, GenerateOpts{})
	if err != nil {
		t.Fatal(err)
	}

	// Should use method_path_slug fallback
	if _, ok := tools["minimal.get_users_id"]; !ok {
		keys := make([]string, 0, len(tools))
		for k := range tools {
			keys = append(keys, k)
		}
		t.Errorf("expected minimal.get_users_id, got %v", keys)
	}
}

func TestGenerateNoPaths(t *testing.T) {
	spec := `
openapi: "3.0.0"
info:
  title: Empty
paths: {}
`
	path := writeSpec(t, spec)
	_, err := Generate(path, GenerateOpts{})
	if err == nil {
		t.Fatal("expected error for spec with no operations")
	}
}

func TestGenerateMissingFile(t *testing.T) {
	_, err := Generate("/nonexistent/spec.yaml", GenerateOpts{})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestGenerateInvalidSpec(t *testing.T) {
	path := writeSpec(t, `not: {valid: [openapi`)
	_, err := Generate(path, GenerateOpts{})
	if err == nil {
		t.Fatal("expected error for spec without paths")
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Pet Store", "pet_store"},
		{"My API v2", "my_api_v2"},
		{"/pets/{{petId}}", "pets_petid"},
		{"  spaces  ", "spaces"},
		{"Already_Valid", "already_valid"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := slugify(tt.input)
			if result != tt.expected {
				t.Errorf("slugify(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
