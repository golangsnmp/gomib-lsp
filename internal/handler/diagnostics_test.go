package handler

import (
	"testing"

	"github.com/golangsnmp/gomib/mib"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestDiagnosticRange(t *testing.T) {
	tests := []struct {
		name string
		diag mib.Diagnostic
		want protocol.Range
	}{
		{
			name: "full range",
			diag: mib.Diagnostic{Line: 5, Column: 10, EndLine: 5, EndColumn: 18},
			want: protocol.Range{
				Start: protocol.Position{Line: 4, Character: 9},
				End:   protocol.Position{Line: 4, Character: 17},
			},
		},
		{
			name: "multi-line range",
			diag: mib.Diagnostic{Line: 5, Column: 10, EndLine: 7, EndColumn: 3},
			want: protocol.Range{
				Start: protocol.Position{Line: 4, Character: 9},
				End:   protocol.Position{Line: 6, Character: 2},
			},
		},
		{
			name: "missing end falls back to point range",
			diag: mib.Diagnostic{Line: 5, Column: 10},
			want: protocol.Range{
				Start: protocol.Position{Line: 4, Character: 9},
				End:   protocol.Position{Line: 4, Character: 9},
			},
		},
		{
			name: "end < start drops back to point range",
			diag: mib.Diagnostic{Line: 5, Column: 10, EndLine: 5, EndColumn: 5},
			want: protocol.Range{
				Start: protocol.Position{Line: 4, Character: 9},
				End:   protocol.Position{Line: 4, Character: 9},
			},
		},
		{
			name: "synthetic zero diagnostic clamps to origin",
			diag: mib.Diagnostic{},
			want: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 0, Character: 0},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diagnosticRange(&tt.diag)
			if got != tt.want {
				t.Errorf("diagnosticRange = %+v, want %+v", got, tt.want)
			}
		})
	}
}
