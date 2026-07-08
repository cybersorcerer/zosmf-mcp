package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.com/ronaldfunk/zosmf-mcp/internal/config"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/mcp"
	"gitlab.com/ronaldfunk/zosmf-mcp/internal/zosmf"
)

type queryCSITool struct {
	clients *zosmf.ClientRegistry
	cfg     *config.Config
}

type queryCSIArgs struct {
	System     string   `json:"system"`
	CSIDataset string   `json:"csi_dataset"`
	Zone       string   `json:"zone"`
	Zones      []string `json:"zones,omitempty"`
	EntryType  string   `json:"entry_type"`
	Entries    []string `json:"entries,omitempty"`
	Filter     string   `json:"filter,omitempty"`
	Subentries []string `json:"subentries,omitempty"`
}

func NewQueryCSITool(clients *zosmf.ClientRegistry, cfg *config.Config) mcp.ToolHandler {
	return &queryCSITool{clients: clients, cfg: cfg}
}

func (t *queryCSITool) Definition() mcp.Tool {
	return mcp.Tool{
		Name: "zosmf_smpe_query_csi",
		Description: "Query the SMP/E Consolidated Software Inventory (CSI) on a z/OS system. " +
			"Returns information about installed SYSMODs, DDDEFs, and other SMP/E entries.",
		InputSchema: mustJSON(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"system": map[string]any{
					"type":        "string",
					"description": "Name of the z/OS system",
				},
				"csi_dataset": map[string]any{
					"type":        "string",
					"description": "CSI dataset name (e.g. SYS1.SMPE.CSI). If omitted, the first CSI dataset from the system configuration is used.",
				},
				"zone": map[string]any{
					"type":        "string",
					"description": "SMP/E zone name (e.g. GLOBAL, or a target/dlib zone). If omitted, the first target zone from the system configuration is used.",
				},
				"entry_type": map[string]any{
					"type":        "string",
					"description": "Entry type to query. Valid types: SYSMOD, DDDEF, TARGETZONE, DLIB, GLOBALZONE, ASSEM, DATA, LMOD, MAC, MOD, SRC, JAR, JARUPD, PROGRAM, DLIBZONE, FEATURE, FMIDSET, HOLDDATA, OPTIONS, ORDER, PRODUCT, UTILITY, ZONESET, and HFS types (AIX1-5, CLIENT1-5, OS21-5, UNIX1-5, WIN1-5).",
				},
				"filter": map[string]any{
					"type":        "string",
					"description": "Entry name filter pattern (supports * wildcard, e.g. ABC* or UI*)",
				},
				"subentries": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
					"description": "List of subentry types to include in results. If omitted, useful defaults are used per entry type. " +
						"SYSMOD defaults: FMID, SMODTYPE, REWORK, RECDATE, RECTIME, APPLY, INSTALLDATE, INSTALLTIME, LASTSUP, DELBY, ERROR, VERSION. " +
						"DDDEF defaults: DATASET, PATH, DATACLAS, MGMTCLAS, STORCLAS, VOLUME. " +
						"PRODUCT defaults: PRODID, VRM, DESCRIPTION, VENDOR, SREL. " +
						"HOLDDATA defaults: HOLDCLASS, HOLDREASON, HOLDDATE, HOLDFIXCAT, HOLDRESOLVER.",
				},
			},
			"required": []string{"system", "entry_type"},
		}),
	}
}

func (t *queryCSITool) Execute(ctx context.Context, arguments json.RawMessage) (mcp.ToolCallResult, error) {
	var args queryCSIArgs
	if err := json.Unmarshal(arguments, &args); err != nil {
		return mcp.ErrorResult("invalid arguments: " + err.Error()), nil
	}

	// Validate entry type
	entryType := strings.ToUpper(args.EntryType)
	validSubs, ok := zosmf.ValidEntryTypes[entryType]
	if !ok {
		types := make([]string, 0, len(zosmf.ValidEntryTypes))
		for t := range zosmf.ValidEntryTypes {
			types = append(types, t)
		}
		return mcp.ErrorResult(fmt.Sprintf("invalid entry_type %q. Valid types: %s", args.EntryType, strings.Join(types, ", "))), nil
	}

	// Validate subentries
	for _, sub := range args.Subentries {
		subUpper := strings.ToUpper(sub)
		found := false
		for _, v := range validSubs {
			if v == subUpper {
				found = true
				break
			}
		}
		if !found {
			return mcp.ErrorResult(fmt.Sprintf("invalid subentry %q for entry type %s. Valid subentries: %s", sub, entryType, strings.Join(validSubs, ", "))), nil
		}
	}

	client, err := t.clients.Get(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	sysCfg, err := t.cfg.SystemByName(args.System)
	if err != nil {
		return mcp.ErrorResult(err.Error()), nil
	}

	csiDataset := args.CSIDataset
	if csiDataset == "" {
		if len(sysCfg.CSIDatasets) == 0 {
			return mcp.ErrorResult(fmt.Sprintf("no csi_dataset provided and system %s has no csi_datasets configured", args.System)), nil
		}
		csiDataset = sysCfg.CSIDatasets[0]
	}

	// Resolve zones: prefer zones array, fall back to zone string, then config default
	var zones []string
	if len(args.Zones) > 0 {
		zones = args.Zones
	} else if args.Zone != "" {
		zones = []string{args.Zone}
	} else {
		if len(sysCfg.TargetZones) == 0 {
			return mcp.ErrorResult(fmt.Sprintf("no zone provided and system %s has no target_zones configured", args.System)), nil
		}
		zones = sysCfg.TargetZones
	}

	// Resolve entries: prefer entries array, fall back to entry_type
	var entries []string
	if len(args.Entries) > 0 {
		entries = args.Entries
	} else {
		entries = []string{entryType}
	}

	// Build filter expression: if user provided a simple name/wildcard, wrap it as ENAME filter
	filter := args.Filter
	if filter != "" && !containsOperator(filter) {
		filter = fmt.Sprintf("ENAME='%s'", filter)
	}

	// Use default subentries if none were explicitly requested
	requestedSubs := args.Subentries
	if len(requestedSubs) == 0 {
		if defaults, ok := zosmf.DefaultSubentries[entryType]; ok {
			requestedSubs = defaults
		}
	}

	// z/OSMF expects subentries as a single comma-separated string in a one-element array
	var subentries []string
	if len(requestedSubs) > 0 {
		subentries = []string{strings.Join(requestedSubs, ",")}
	}

	resp, err := client.QueryCSI(zosmf.CSIQueryRequest{
		CSIDataset: csiDataset,
		Zones:      zones,
		Entries:    entries,
		Filter:     filter,
		Subentries: subentries,
	})
	if err != nil {
		return mcp.ErrorResult(fmt.Sprintf("CSI query failed: %v", err)), nil
	}

	data, _ := json.MarshalIndent(resp, "", "  ")
	return mcp.TextResult(string(data)), nil
}

// containsOperator checks if a filter string already contains an SMP/E CSI filter operator.
func containsOperator(s string) bool {
	return strings.ContainsAny(s, "=!<>|")
}
