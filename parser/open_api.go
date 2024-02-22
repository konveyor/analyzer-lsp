package parser

import "github.com/swaggest/openapi-go/openapi3"

var (
	stringtype openapi3.SchemaType = openapi3.SchemaTypeString
	arraytype  openapi3.SchemaType = openapi3.SchemaTypeArray
	objecttype openapi3.SchemaType = openapi3.SchemaTypeObject
	numbertype openapi3.SchemaType = openapi3.SchemaTypeInteger
)

func CreateSchema() (openapi3.ComponentsSchemas, error) {
	schema := openapi3.ComponentsSchemas{
		MapOfSchemaOrRefValues: map[string]openapi3.SchemaOrRef{},
	}

	schema.MapOfSchemaOrRefValues["rule"] = openapi3.SchemaOrRef{
		Schema: &openapi3.Schema{
			Type: &objecttype,
			Properties: map[string]openapi3.SchemaOrRef{
				"ruleID": {
					Schema: &openapi3.Schema{
						Type: &stringtype,
					},
				},
				"description": {
					Schema: &openapi3.Schema{
						Type: &stringtype,
					},
				},
				"labels": {
					Schema: &openapi3.Schema{
						Type: &arraytype,
						Items: &openapi3.SchemaOrRef{
							Schema: &openapi3.Schema{
								Type: &stringtype,
							},
						},
					},
				},
				"effort": {
					Schema: &openapi3.Schema{
						Type: &numbertype,
					},
				},
				"category": {
					Schema: &openapi3.Schema{
						Type: &stringtype,
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
						Type: &arraytype,
						Properties: map[string]openapi3.SchemaOrRef{
							"name": {
								Schema: &openapi3.Schema{
									Type: &stringtype,
								},
							},
							"defaultValue": {
								Schema: &openapi3.Schema{
									Type: &stringtype,
								},
							},
							"nameOfCaptureGroup": {
								Schema: &openapi3.Schema{
									Type: &stringtype,
								},
							},
						},
					},
				},
				"message": {
					Schema: &openapi3.Schema{
						Type: &stringtype,
					},
				},
				"tag": {
					Schema: &openapi3.Schema{
						Type: &arraytype,
						Items: &openapi3.SchemaOrRef{
							Schema: &openapi3.Schema{
								Type: &stringtype,
							},
						},
					},
				},
				// TODO: here we need to add each capbability and/or
				// We should be able to use references.
				"when": {},
			},
		},
	}

	schema.MapOfSchemaOrRefValues["rulesets"] = openapi3.SchemaOrRef{
		Schema: &openapi3.Schema{
			Type: &objecttype,
			Properties: map[string]openapi3.SchemaOrRef{
				"name": {
					Schema: &openapi3.Schema{
						Type: &stringtype,
					},
				},
				"description": {
					Schema: &openapi3.Schema{
						Type: &stringtype,
					},
				},
				"labels": {
					Schema: &openapi3.Schema{
						Type: &arraytype,
						Items: &openapi3.SchemaOrRef{
							Schema: &openapi3.Schema{
								Type: &stringtype,
							},
						},
					},
				},
				"tags": {
					Schema: &openapi3.Schema{
						Type: &arraytype,
						Items: &openapi3.SchemaOrRef{
							Schema: &openapi3.Schema{
								Type: &stringtype,
							},
						},
					},
				},
				"rules": {
					Schema: &openapi3.Schema{
						Type: &arraytype,
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
