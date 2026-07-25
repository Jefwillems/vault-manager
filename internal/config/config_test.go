package config

import "testing"

func TestParseLayoutDirs(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{name: "empty", in: "", want: nil},
		{name: "blanks only", in: " , ,\t", want: nil},
		{name: "single", in: "Projects", want: []string{"Projects"}},
		{name: "multiple trimmed", in: "Projects, Reading ,1-1-Prep", want: []string{"Projects", "Reading", "1-1-Prep"}},
		{name: "nested", in: "Areas/Work", want: []string{"Areas/Work"}},
		{name: "cleaned", in: "Areas//Work/", want: []string{"Areas/Work"}},
		{name: "absolute rejected", in: "/etc", wantErr: true},
		{name: "dotdot rejected", in: "../escape", wantErr: true},
		{name: "dotdot nested rejected", in: "Projects/../../etc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLayoutDirs(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %v", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseLayoutDirs(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseLayoutDirs(%q) = %v, want %v", tt.in, got, tt.want)
				}
			}
		})
	}
}
