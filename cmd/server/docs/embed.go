package docs

import (
	_ "embed"
	"encoding/json"
)

//go:embed swagger.json
var swaggerJSON []byte

// SwaggerSpec represents the structure of our swagger.json file
type SwaggerSpec struct {
	Paths map[string]map[string]PathInfo `json:"paths"`
}

// PathInfo contains information about an API endpoint
type PathInfo struct {
	Summary     string                 `json:"summary"`
	Description string                 `json:"description"`
	Tags        []string               `json:"tags"`
	Parameters  []interface{}          `json:"parameters"`
	Responses   map[string]interface{} `json:"responses"`
}

// GetSwaggerSpec returns the parsed swagger specification
func GetSwaggerSpec() (*SwaggerSpec, error) {
	var spec SwaggerSpec
	if err := json.Unmarshal(swaggerJSON, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}
