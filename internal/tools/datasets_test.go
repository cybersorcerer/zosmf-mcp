package tools

import (
	"encoding/json"
	"testing"
)

func TestReadDatasetArgs_OmitsSearchByDefault(t *testing.T) {
	raw := []byte(`{"system":"PROD","dataset":"HLQ.SEQ"}`)
	var args readDatasetArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatal(err)
	}
	if args.Search != "" {
		t.Errorf("Search = %q, want empty", args.Search)
	}
	if args.Insensitive != nil {
		t.Error("Insensitive should be nil when not supplied")
	}
}

func TestReadDatasetArgs_AcceptsSearchAndInsensitive(t *testing.T) {
	raw := []byte(`{"system":"PROD","dataset":"HLQ.PDS(M)","search":"IKJEF","insensitive":false,"maxreturnsize":50}`)
	var args readDatasetArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatal(err)
	}
	if args.Search != "IKJEF" {
		t.Errorf("Search = %q", args.Search)
	}
	if args.Insensitive == nil || *args.Insensitive != false {
		t.Errorf("Insensitive = %v, want pointer to false", args.Insensitive)
	}
	if args.MaxReturnSize != 50 {
		t.Errorf("MaxReturnSize = %d", args.MaxReturnSize)
	}
}

func TestReadDatasetArgs_InsensitiveTrueExplicit(t *testing.T) {
	raw := []byte(`{"system":"PROD","dataset":"HLQ.SEQ","search":"foo","insensitive":true}`)
	var args readDatasetArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatal(err)
	}
	if args.Insensitive == nil || *args.Insensitive != true {
		t.Errorf("Insensitive = %v, want pointer to true", args.Insensitive)
	}
}
