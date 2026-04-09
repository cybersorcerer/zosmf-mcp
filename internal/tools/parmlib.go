package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

// --- Parmlib Validate ---

type parmlibValidateTool struct {
	clients *zosmf.ClientRegistry
}

type parmlibValidateArgs struct {
	System      string `json:"system"`
	MemberType  string `json:"member_type"`
	Member      string `json:"member,omitempty"`
	Dataset     string `json:"dataset,omitempty"`
	Volser      string `json:"volser,omitempty"`
	LoadMember  string `json:"load_member,omitempty"`
	LoadDataset string `json:"load_dataset,omitempty"`
	LoadVolser  string `json:"load_volser,omitempty"`
	Deep        bool   `json:"deep,omitempty"`
	RemSystem   string `json:"rem_system,omitempty"`
	Content     string `json:"content,omitempty"`
}

func NewParmlibValidateTool(clients *zosmf.ClientRegistry) mcp.ToolHandler {
	return &parmlibValidateTool{clients: clients}
}

func (t *parmlibValidateTool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_parmlib_validate",
		Description: "Validate the syntax of z/OS parmlib members using the z/OSMF Parmlib Management REST API. " +
			"Specify member_type without the xx suffix (e.g. 'BPXPRM', 'IEASYS', 'IKJTSO'). " +
			"Without optional parameters, validates all active members of the given type. " +
			"Use 'member' + 'dataset' to validate a specific member. " +
			"Use 'content' to validate inline parmlib text. " +
			"Use 'deep=true' with member_type LOAD to validate all parmlib types deeply. " +
			"Supported types: ALLOC, AUTOR, AXR, BPXPRM, CEAPRM, CEEPARM, CLOCK, COMMND, " +
			"CONSOL, COUPLE, CUNUNI, DEVSUP, DIAG, EXIT, FXEPRM, GRSCNF, GRSRNL, IEAFIX, " +
			"IEALPA, IEAPAK, IEASVC, IEASYM, IEASYS, IEFOPZ, IEFSSN, IFAPRD, IGDSMS, IGGCAT, " +
			"IKJTSO, IQPPRM, IRRPRM, IXGCNF, IZUPRM, LOAD, LPALST, PROG, SCHED, VATLST.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system (e.g. 'MAINT')",
				},
				"member_type": map[string]any{
					"type":        "string",
					"description": "Parmlib member type without xx suffix (e.g. 'BPXPRM', 'IEASYS', 'LOAD')",
				},
				"member": map[string]any{
					"type":        "string",
					"description": "Specific member name to validate (e.g. 'BPXPRM00'). Requires 'dataset'.",
				},
				"dataset": map[string]any{
					"type":        "string",
					"description": "Dataset containing the specific member (e.g. 'SYS1.PARMLIB')",
				},
				"volser": map[string]any{
					"type":        "string",
					"description": "Volser for dataset if not cataloged",
				},
				"load_member": map[string]any{
					"type":        "string",
					"description": "LOADxx member name for type resolution (e.g. 'LOADAC')",
				},
				"load_dataset": map[string]any{
					"type":        "string",
					"description": "Dataset containing the LOADxx member",
				},
				"load_volser": map[string]any{
					"type":        "string",
					"description": "Volser for load_dataset if not cataloged",
				},
				"deep": map[string]any{
					"type":        "boolean",
					"description": "Validate all parmlib types deeply. Only valid with member_type 'LOAD'.",
				},
				"rem_system": map[string]any{
					"type":        "string",
					"description": "Remote system name in the sysplex to validate on",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Inline parmlib content to validate (instead of reading from a dataset)",
				},
			},
			"required": []string{"system", "member_type"},
		}),
	}
}

func (t *parmlibValidateTool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args parmlibValidateArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	result, err := client.ValidateParmlib(zosmf.ParmlibValidateOptions{
		MemberType:  args.MemberType,
		Member:      args.Member,
		Dataset:     args.Dataset,
		Volser:      args.Volser,
		LoadMember:  args.LoadMember,
		LoadDataset: args.LoadDataset,
		LoadVolser:  args.LoadVolser,
		Deep:        args.Deep,
		System:      args.RemSystem,
		Content:     args.Content,
	})
	if err != nil {
		return mcp.ErrorResult("failed to validate parmlib: " + err.Error()), nil
	}

	return mcp.TextResult(formatParmlibResult(result)), nil
}

func formatParmlibResult(r *zosmf.ParmlibValidateResult) string {
	var sb strings.Builder

	mark := "PASSED"
	if r.Result == "failed" {
		mark = "FAILED"
	}
	fmt.Fprintf(&sb, "Result: %s (%d validated, %d failed)\n", mark, r.NumberOfMembers, r.NumberOfFailedMembers)

	if len(r.ParmlibDatasets) > 0 {
		sb.WriteString("\nParmlib Datasets:\n")
		for _, ds := range r.ParmlibDatasets {
			fmt.Fprintf(&sb, "  %s", ds.Dataset)
			if ds.Volser != "" && ds.Volser != "null" {
				fmt.Fprintf(&sb, "  (volser: %s)", ds.Volser)
			}
			sb.WriteString("\n")
		}
	}

	if r.Details == nil {
		return sb.String()
	}

	// Try single member result
	var single struct {
		Member           string               `json:"member"`
		Dataset          string               `json:"dataset"`
		ValidationResult string               `json:"validationResult"`
		Warning          string               `json:"warning"`
		NumOfFailure     int                  `json:"numOfFailure"`
		Failures         []parmlibFailureItem `json:"failures"`
	}
	if err := json.Unmarshal(r.Details, &single); err == nil && single.ValidationResult != "" {
		sb.WriteString("\n")
		writeParmlibMemberResult(&sb, "", single.Member, single.Dataset, single.ValidationResult, single.Warning, single.Failures)
		return sb.String()
	}

	// Try multi-member result (map of type → members)
	var multi map[string]json.RawMessage
	if err := json.Unmarshal(r.Details, &multi); err != nil {
		sb.WriteString("\n")
		sb.Write(r.Details)
		return sb.String()
	}

	sb.WriteString("\n")
	for typeName, typeRaw := range multi {
		fmt.Fprintf(&sb, "  %s\n", typeName)

		var members []json.RawMessage
		if err := json.Unmarshal(typeRaw, &members); err != nil {
			var warn struct {
				Warning string `json:"warning"`
			}
			if err2 := json.Unmarshal(typeRaw, &warn); err2 == nil && warn.Warning != "" {
				fmt.Fprintf(&sb, "    ! %s\n", warn.Warning)
			}
			continue
		}

		for _, memberRaw := range members {
			var m struct {
				Member           string               `json:"member"`
				Dataset          string               `json:"dataset"`
				ValidationResult string               `json:"validationResult"`
				Warning          string               `json:"warning"`
				NumOfFailure     int                  `json:"numOfFailure"`
				Failures         []parmlibFailureItem `json:"failures"`
			}
			if err := json.Unmarshal(memberRaw, &m); err != nil {
				continue
			}
			writeParmlibMemberResult(&sb, "  ", m.Member, m.Dataset, m.ValidationResult, m.Warning, m.Failures)
		}
	}

	return sb.String()
}

type parmlibFailureItem struct {
	Index    int `json:"index"`
	Location struct {
		Line     int `json:"line"`
		Position int `json:"position"`
	} `json:"location"`
	CurrentField  string   `json:"currentField"`
	ExpectedField []string `json:"expectedField"`
	AfterField    string   `json:"afterField"`
	Message       string   `json:"message"`
}

func writeParmlibMemberResult(sb *strings.Builder, indent, member, dataset, validationResult, warning string, failures []parmlibFailureItem) {
	mark := "✓"
	if validationResult == "failed" {
		mark = "✗"
	} else if validationResult == "N/A" {
		mark = "-"
	}

	line := member
	if dataset != "" {
		line += "  " + dataset
	}
	if len(failures) > 0 {
		fmt.Fprintf(sb, "%s  %s %-30s (%d failure(s))\n", indent, mark, line, len(failures))
	} else {
		fmt.Fprintf(sb, "%s  %s %s\n", indent, mark, line)
	}

	if warning != "" {
		fmt.Fprintf(sb, "%s    ! %s\n", indent, warning)
	}

	for _, f := range failures {
		fmt.Fprintf(sb, "%s    [Line %d, Pos %d] %s after %s → expected: %s\n",
			indent, f.Location.Line, f.Location.Position,
			f.CurrentField, f.AfterField,
			strings.Join(f.ExpectedField, ", "))
	}
}
