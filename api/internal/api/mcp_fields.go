package api

import (
	"context"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/oas"
)

type mcpCustomField struct {
	ID              uuid.UUID `json:"id"`
	Object          string    `json:"object" jsonschema:"conversation or contact"`
	Key             string    `json:"key" jsonschema:"the key of the value in a conversation's fields or a contact's attributes"`
	Name            string    `json:"name"`
	Type            string    `json:"type" jsonschema:"text (a string), number, date (YYYY-MM-DD), select (one of options) or checkbox (true or false)"`
	Options         []string  `json:"options,omitempty" jsonschema:"the values a select field takes"`
	RequiredOnClose bool      `json:"required_on_close,omitempty" jsonschema:"closing a conversation without a value is refused"`
}

type listCustomFieldsIn struct {
	Object string `json:"object,omitempty" jsonschema:"only the fields of conversation or contact"`
}

type setConversationFieldsIn struct {
	ConversationID uuid.UUID      `json:"conversation_id"`
	Fields         map[string]any `json:"fields" jsonschema:"values by key from list_custom_fields; null clears a value, keys not named are kept"`
}

var mcpFieldTools = []mcpTool{
	defineTool("list_custom_fields", "List custom fields",
		"Lists the workspace's custom fields of conversations and contacts with their key, type and options, for set_conversation_fields and the fields filter of search_conversations. Maps to GET /v1/custom-fields.",
		readOnly, map[string][]any{"object": {"conversation", "contact"}},
		mcpTool{ops: []string{"ListCustomFields"}, allow: func(p principal) bool { return readsFields(p) == nil }},
		func(ctx context.Context, c *mcpCall, in listCustomFieldsIn) (mcpPage[mcpCustomField], error) {
			res, err := c.s.ListCustomFields(ctx, oas.ListCustomFieldsRequestObject{Params: oas.ListCustomFieldsParams{Object: optString[oas.CustomFieldObject](in.Object)}})
			if err != nil {
				return mcpPage[mcpCustomField]{}, err
			}
			out := mcpPage[mcpCustomField]{Items: []mcpCustomField{}}
			for _, f := range res.(oas.ListCustomFields200JSONResponse).Items {
				out.Items = append(out.Items, mcpCustomField{
					ID: f.Id, Object: string(f.Object), Key: f.Key, Name: f.Name, Type: string(f.Type), Options: f.Options, RequiredOnClose: f.RequiredOnClose,
				})
			}
			return out, nil
		}),
	defineTool("set_conversation_fields", "Set conversation fields",
		"Sets custom field values of a conversation; keys come from list_custom_fields and each value must fit the field's type. Maps to PATCH /v1/conversations/{id} with fields.",
		idempotent, nil, mcpTool{ops: []string{"UpdateConversation"}},
		func(ctx context.Context, c *mcpCall, in setConversationFieldsIn) (mcpConversation, error) {
			if len(in.Fields) == 0 {
				return mcpConversation{}, errValidation("fields must name at least one key")
			}
			f := oas.CustomFieldValuesUpdate(in.Fields)
			return c.updateConversation(ctx, in.ConversationID, oas.ConversationUpdate{Fields: &f})
		}),
}
