package parser

import (
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/swaggest/openapi-go/openapi3"
)

func CreateSchema() (openapi3.ComponentsSchemas, error) {
	schema := openapi3.ComponentsSchemas{
		MapOfSchemaOrRefValues: map[string]openapi3.SchemaOrRef{},
	}

	schema.MapOfSchemaOrRefValues["rule"] = openapi3.SchemaOrRef{
		Schema: &openapi3.Schema{
			Type: &provider.SchemaTypeObject,
			Properties: map[string]openapi3.SchemaOrRef{
				"ruleID": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeString,
					},
				},
				"description": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeString,
					},
				},
				"labels": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeArray,
						Items: &openapi3.SchemaOrRef{
							Schema: &openapi3.Schema{
								Type: &provider.SchemaTypeString,
							},
						},
					},
				},
				"effort": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeNumber,
					},
				},
				"category": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeString,
						OneOf: []openapi3.SchemaOrRef{
							{
								Schema: &openapi3.Schema{
									Enum: []interface{}{
										"potential",
										"optional",
										"mandatory",
									},
								},
							},
						},
					},
				},
				"customVariable": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeArray,
						Properties: map[string]openapi3.SchemaOrRef{
							"name": {
								Schema: &openapi3.Schema{
									Type: &provider.SchemaTypeString,
								},
							},
							"defaultValue": {
								Schema: &openapi3.Schema{
									Type: &provider.SchemaTypeString,
								},
							},
							"nameOfCaptureGroup": {
								Schema: &openapi3.Schema{
									Type: &provider.SchemaTypeString,
								},
							},
						},
					},
				},
				"message": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeString,
					},
				},
				"tag": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeArray,
						Items: &openapi3.SchemaOrRef{
							Schema: &openapi3.Schema{
								Type: &provider.SchemaTypeString,
							},
						},
					},
				},
				// We will override this, with the capabilties from the providers
				"when": {},
			},
		},
	}

	schema.MapOfSchemaOrRefValues["rulesets"] = openapi3.SchemaOrRef{
		Schema: &openapi3.Schema{
			Type: &provider.SchemaTypeObject,
			Properties: map[string]openapi3.SchemaOrRef{
				"name": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeString,
					},
				},
				"description": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeString,
					},
				},
				"labels": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeArray,
						Items: &openapi3.SchemaOrRef{
							Schema: &openapi3.Schema{
								Type: &provider.SchemaTypeString,
							},
						},
					},
				},
				"tags": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeArray,
						Items: &openapi3.SchemaOrRef{
							Schema: &openapi3.Schema{
								Type: &provider.SchemaTypeString,
							},
						},
					},
				},
				"rules": {
					Schema: &openapi3.Schema{
						Type: &provider.SchemaTypeArray,
						Items: &openapi3.SchemaOrRef{
							SchemaReference: &openapi3.SchemaReference{
								Ref: "#/components/schemas/rule",
							},
						},
					},
				},
			},
		},
	}

	return schema, nil

}
