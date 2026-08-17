package resources

import "testing"

func TestParseURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want Ref
	}{
		{
			name: "systems inventory",
			uri:  "zosmf:///systems",
			want: Ref{Kind: KindSystems},
		},
		{
			name: "sequential dataset",
			uri:  "zosmf://S0W1/dataset/USER.DATA.SEQ",
			want: Ref{Kind: KindDataset, System: "S0W1", Dataset: "USER.DATA.SEQ"},
		},
		{
			name: "pds member",
			uri:  "zosmf://S0W1/dataset/SYS1.PARMLIB(IEASYS00)",
			want: Ref{Kind: KindDataset, System: "S0W1", Dataset: "SYS1.PARMLIB(IEASYS00)"},
		},
		{
			name: "dataset lower case is upper-cased",
			uri:  "zosmf://s0w1/dataset/user.data",
			want: Ref{Kind: KindDataset, System: "S0W1", Dataset: "USER.DATA"},
		},
		{
			name: "uss file",
			uri:  "zosmf://S0W1/uss/etc/profile",
			want: Ref{Kind: KindUSS, System: "S0W1", Path: "/etc/profile"},
		},
		{
			name: "uss path keeps its case",
			uri:  "zosmf://S0W1/uss/u/ibmuser/MyScript.sh",
			want: Ref{Kind: KindUSS, System: "S0W1", Path: "/u/ibmuser/MyScript.sh"},
		},
		{
			name: "spool file",
			uri:  "zosmf://S0W1/job/MYJOB/JOB12345/spool/2",
			want: Ref{Kind: KindSpool, System: "S0W1", JobName: "MYJOB", JobID: "JOB12345", FileID: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseURI(tt.uri)
			if err != nil {
				t.Fatalf("ParseURI(%q) returned error: %v", tt.uri, err)
			}
			if got != tt.want {
				t.Errorf("ParseURI(%q) = %+v, want %+v", tt.uri, got, tt.want)
			}
		})
	}
}

func TestParseURIRejects(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"unknown scheme", "http://S0W1/dataset/USER.DATA"},
		{"unknown kind", "zosmf://S0W1/volume/VOL001"},
		{"dataset without name", "zosmf://S0W1/dataset/"},
		{"uss without path", "zosmf://S0W1/uss"},
		{"spool with non-numeric id", "zosmf://S0W1/job/MYJOB/JOB12345/spool/abc"},
		{"spool truncated", "zosmf://S0W1/job/MYJOB/JOB12345"},
		{"system-scoped without system", "zosmf:///dataset/USER.DATA"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseURI(tt.uri); err == nil {
				t.Errorf("ParseURI(%q) succeeded, want error", tt.uri)
			}
		})
	}
}

func TestBuildersRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want Ref
	}{
		{"dataset", DatasetURI("S0W1", "SYS1.PARMLIB(IEASYS00)"),
			Ref{Kind: KindDataset, System: "S0W1", Dataset: "SYS1.PARMLIB(IEASYS00)"}},
		{"uss", USSURI("S0W1", "/etc/profile"),
			Ref{Kind: KindUSS, System: "S0W1", Path: "/etc/profile"}},
		{"spool", SpoolURI("S0W1", "MYJOB", "JOB12345", 2),
			Ref{Kind: KindSpool, System: "S0W1", JobName: "MYJOB", JobID: "JOB12345", FileID: 2}},
		{"systems", SystemsURI, Ref{Kind: KindSystems}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseURI(tt.uri)
			if err != nil {
				t.Fatalf("ParseURI(%q): %v", tt.uri, err)
			}
			if got != tt.want {
				t.Errorf("round trip of %q = %+v, want %+v", tt.uri, got, tt.want)
			}
		})
	}
}
