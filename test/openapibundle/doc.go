// Package openapibundle pins api/openapi.bundled.json, the self-contained
// OpenAPI document every release publishes as shift2eu-server_openapi.json
// (issue #139, ADR 0019): the committed file is exactly what
// test/internal/openapibundle makes of api/openapi.json and the report schemas
// it refers to, and every $ref it holds is a pointer into the document itself.
//
// go test ./test/openapibundle -update (make openapi) rewrites the file.
package openapibundle
