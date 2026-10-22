package server

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

type specOperation struct {
	OperationID string                     `json:"operationId"`
	Summary     string                     `json:"summary"`
	Responses   map[string]json.RawMessage `json:"responses"`
	RequestBody *struct {
		Content map[string]struct {
			Example  json.RawMessage `json:"example"`
			Examples json.RawMessage `json:"examples"`
		} `json:"content"`
	} `json:"requestBody"`
}

type openAPISpec struct {
	OpenAPI string                              `json:"openapi"`
	Paths   map[string]map[string]specOperation `json:"paths"`
}

var httpMethods = map[string]bool{"get": true, "put": true, "post": true, "delete": true, "patch": true, "head": true, "options": true}

func loadSpec(t *testing.T) openAPISpec {
	t.Helper()
	raw, err := os.ReadFile("../../docs/api/openapi.json")
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var spec openAPISpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	return spec
}

// TestSpecMatchesRoutes fails when a served route is undocumented or a
// documented route is not served (docs/api/openapi.json vs Routes).
func TestSpecMatchesRoutes(t *testing.T) {
	spec := loadSpec(t)
	if !regexp.MustCompile(`^3\.\d+\.\d+$`).MatchString(spec.OpenAPI) {
		t.Fatalf("openapi = %q, want 3.x", spec.OpenAPI)
	}

	var served, documented []string
	for _, route := range Routes {
		for _, method := range route.Methods {
			served = append(served, method+" "+route.Path)
		}
	}
	ids := map[string]bool{}
	for path, item := range spec.Paths {
		for method, op := range item {
			if !httpMethods[method] {
				continue
			}
			key := strings.ToUpper(method) + " " + path
			documented = append(documented, key)
			if op.OperationID == "" || op.Summary == "" {
				t.Errorf("%s lacks operationId or summary", key)
			}
			if ids[op.OperationID] {
				t.Errorf("duplicate operationId %s", op.OperationID)
			}
			ids[op.OperationID] = true
			has2xx := false
			for code := range op.Responses {
				has2xx = has2xx || strings.HasPrefix(code, "2")
			}
			if !has2xx {
				t.Errorf("%s lacks a 2xx response", key)
			}
			if op.RequestBody != nil {
				media, ok := op.RequestBody.Content["application/json"]
				if ok && media.Example == nil && media.Examples == nil {
					t.Errorf("%s request body lacks an example", key)
				}
			}
		}
	}
	sort.Strings(served)
	sort.Strings(documented)
	if strings.Join(served, "\n") != strings.Join(documented, "\n") {
		t.Fatalf("routes and spec differ\nserved:\n%s\n\ndocumented:\n%s",
			strings.Join(served, "\n"), strings.Join(documented, "\n"))
	}
}
