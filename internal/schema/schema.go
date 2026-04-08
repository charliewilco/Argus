package schema

type FieldType string

const (
	FieldText          FieldType = "text"
	FieldSecret        FieldType = "secret"
	FieldSelect        FieldType = "select"
	FieldDynamicSelect FieldType = "dynamic_select"
	FieldBoolean       FieldType = "boolean"
	FieldNumber        FieldType = "number"
)

type Option struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type Field struct {
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	Description string    `json:"description,omitempty"`
	Type        FieldType `json:"type"`
	Required    bool      `json:"required"`
	Options     []Option  `json:"options,omitempty"`
	OptionsPath string    `json:"optionsPath,omitempty"`
	DependsOn   string    `json:"dependsOn,omitempty"`
}
