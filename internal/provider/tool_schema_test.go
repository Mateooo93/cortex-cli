package provider

import "testing"

func TestOaiParamSchema_NestedArrayObject(t *testing.T) {
	schema := oaiParamSchema(ToolParam{
		Type:        "array",
		Description: "edits",
		Items: &ToolParam{Type: "object", Properties: map[string]ToolParam{
			"oldText": {Type: "string", Description: "old", Required: true},
			"newText": {Type: "string", Description: "new", Required: true},
		}},
	})
	if schema["type"] != "array" {
		t.Fatalf("expected array type, got %#v", schema["type"])
	}
	items, ok := schema["items"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected object items schema, got %#v", schema["items"])
	}
	props, ok := items["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested properties, got %#v", items)
	}
	if props["oldText"] == nil || props["newText"] == nil {
		t.Fatalf("missing nested edit props: %#v", props)
	}
	req, ok := items["required"].([]string)
	if !ok || len(req) != 2 {
		t.Fatalf("expected oldText/newText required list, got %#v", items["required"])
	}
}
