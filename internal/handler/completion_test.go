package handler

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestPartialWordBeforeCursor(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"Disp", "Disp"},
		{"STATUS cur", "cur"},
		{"SYNTAX ", ""},
		{"FROM SNMPv2-TC", "SNMPv2-TC"},
		{"::= { sys", "sys"},
	}

	for _, tt := range tests {
		got := partialWordBeforeCursor(tt.text)
		if got != tt.want {
			t.Errorf("partialWordBeforeCursor(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestPrefixFilter(t *testing.T) {
	kind := protocol.CompletionItemKindVariable
	items := []protocol.CompletionItem{
		{Label: "sysDescr", Kind: &kind},
		{Label: "sysName", Kind: &kind},
		{Label: "ifDescr", Kind: &kind},
		{Label: "SysContact", Kind: &kind},
	}

	tests := []struct {
		prefix string
		want   int
	}{
		{"sys", 3}, // sysDescr, sysName, SysContact (case-insensitive)
		{"SYS", 3},
		{"if", 1},
		{"xyz", 0},
		{"", 4},
	}

	for _, tt := range tests {
		got := prefixFilter(items, tt.prefix)
		if len(got) != tt.want {
			t.Errorf("prefixFilter(%q) returned %d items, want %d", tt.prefix, len(got), tt.want)
		}
	}
}
