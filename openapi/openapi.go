// Package openapi embeds Validly's OpenAPI 3.1 document.
package openapi

import _ "embed"

// Spec is the OpenAPI 3.1 document served at /openapi.json.
//
//go:embed openapi.json
var Spec []byte
