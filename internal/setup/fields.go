// Package setup builds and edits the configuration files that zosmf-mcp
// reads. One field catalog feeds the interactive wizard, the command-line
// flags and the JSON question list, so the three cannot drift apart.
package setup

import "encoding/json"

type Kind string

const (
	KindString Kind = "string"
	KindInt    Kind = "int"
	KindBool   Kind = "bool"
)

type Cardinality string

const (
	One  Cardinality = "one"
	Many Cardinality = "many"
)

// Field describes one answer the setup needs. ID is the flag name without
// dashes; YAMLKey is where the answer goes in the configuration file.
type Field struct {
	ID          string      `json:"id"`
	YAMLKey     string      `json:"yaml_key"`
	Label       string      `json:"label"`
	Help        string      `json:"help"`
	Kind        Kind        `json:"type"`
	Cardinality Cardinality `json:"cardinality"`
	Required    bool        `json:"required"`
	Default     string      `json:"default,omitempty"`
	Example     any         `json:"example,omitempty"`
	Group       string      `json:"group"`
}

// ConfigFields is the catalog for `init config`.
func ConfigFields() []Field {
	return []Field{
		{
			ID: "name", YAMLKey: "name", Label: "System name",
			Help: "Name used as the 'system' argument in tool calls. Uppercased.",
			Kind: KindString, Cardinality: One, Required: true,
			Example: "MAINT", Group: "system",
		},
		{
			ID: "host", YAMLKey: "host", Label: "z/OSMF base URL",
			Help: "Including the scheme, for example https://zos.example.com:443",
			Kind: KindString, Cardinality: One, Required: true,
			Example: "https://zos.example.com:443", Group: "system",
		},
		{
			ID: "port", YAMLKey: "port", Label: "Port",
			Help: "Only needed when the URL carries no port.",
			Kind: KindInt, Cardinality: One, Group: "system", Example: 443,
		},
		{
			ID: "user", YAMLKey: "user", Label: "TSO user id",
			Help: "The z/OSMF user. Its password belongs in the environment, never here.",
			Kind: KindString, Cardinality: One, Required: true,
			Example: "IBMUSER", Group: "system",
		},
		{
			ID: "csi", YAMLKey: "csi", Label: "CSI datasets",
			Help: "SMP/E tools use the first entry when a call names no csi_dataset.",
			Kind: KindString, Cardinality: Many,
			Example: []string{"ZOS31.SMPE.GLOBAL.CSI"}, Group: "system",
		},
		{
			ID: "zones", YAMLKey: "zones", Label: "SMP/E zones",
			Help: "All entries are the default when a call names no zone.",
			Kind: KindString, Cardinality: Many,
			Example: []string{"MVST100", "MVSD100"}, Group: "system",
		},
		{
			ID: "default-zones", YAMLKey: "defaultZones", Label: "Default zones for vscode-smpe",
			Help: "Read by vscode-smpe, ignored by this server. Usually the same as zones.",
			Kind: KindString, Cardinality: Many, Group: "system",
		},
		{
			ID: "tso-account", YAMLKey: "tso_account", Label: "TSO account number",
			Help: "zosmf_tso_command fails without it.",
			Kind: KindString, Cardinality: One, Example: "ACCT#", Group: "system",
		},
		{
			ID: "tso-proc", YAMLKey: "tso_proc", Label: "TSO logon procedure",
			Help: "zosmf_tso_command fails without it.",
			Kind: KindString, Cardinality: One, Example: "IZUFPROC", Group: "system",
		},
		{
			ID: "reject-unauthorized", YAMLKey: "rejectUnauthorized", Label: "Verify the TLS certificate",
			Help: "false accepts a self-signed z/OSMF certificate. Only for test systems.",
			Kind: KindBool, Cardinality: One, Default: "true", Group: "system",
		},
		{
			ID: "ca-file", YAMLKey: "ca_file", Label: "CA certificate file",
			Help: "PEM file for a self-signed z/OSMF certificate — the alternative to not verifying.",
			Kind: KindString, Cardinality: One, Group: "system",
		},
		{
			ID: "home", YAMLKey: "home", Label: "USS home directory",
			Help: "Used by vscpdf and nvpdf, not by this server.",
			Kind: KindString, Cardinality: One, Example: "/u/ibmuser", Group: "system",
		},
	}
}

// CatalogJSON renders a catalog for an agent that has to ask the questions.
func CatalogJSON(command string, fields []Field) ([]byte, error) {
	return json.MarshalIndent(struct {
		Command string  `json:"command"`
		Fields  []Field `json:"fields"`
	}{command, fields}, "", "  ")
}
