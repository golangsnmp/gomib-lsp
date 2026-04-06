package handler

import (
	"fmt"
	"strings"

	"github.com/golangsnmp/gomib/mib"
	"github.com/golangsnmp/gomib/syntax"
	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func (s *Server) textDocumentHover(ctx *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	uri := params.TextDocument.URI

	s.mu.Lock()
	doc := s.documents[uri]
	m := s.mib
	s.mu.Unlock()

	if doc == nil || m == nil {
		return nil, nil
	}

	offset, ok := lspOffset(doc, int(params.Position.Line), int(params.Position.Character))
	if !ok {
		return nil, nil
	}

	if doc.cst == nil {
		return nil, nil
	}

	word := tokenText(doc, offset)
	if word == "" {
		return nil, nil
	}

	// Build a context prefix from CST-level span info.
	var prefix string
	{
		cctx := syntax.CursorContextAt(doc.cst, offset)

		if cctx.ImportGroup != nil {
			modTok := cctx.ImportGroup.Module
			if !modTok.IsZero() {
				declared := doc.content[modTok.Span.Start:modTok.Span.End]
				if mod := moduleForPosition(doc, m); mod != nil {
					if src := mod.ImportSource(word); src != nil && src.Name() != declared {
						prefix = fmt.Sprintf("*Imported from* **%s** (via %s)\n\n---\n\n", src.Name(), declared)
					} else {
						prefix = fmt.Sprintf("*Imported from* **%s**\n\n---\n\n", declared)
					}
				}
			}
		} else if cctx.OidValue != nil {
			if nd := m.Node(word); nd != nil {
				if oid := nd.OID(); len(oid) > 0 {
					prefix = fmt.Sprintf("*OID parent:* `%s`\n\n---\n\n", oid)
				}
			}
		}
	}

	md := symbolHover(m, word)
	if md == "" {
		return nil, nil
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: prefix + md,
		},
	}, nil
}

// symbolHover returns Markdown hover text for a MIB symbol name.
func symbolHover(m *mib.Mib, name string) string {
	// Check keyword lookups before symbol resolution.
	if info, ok := mib.MacroDescription(name); ok {
		return macroHover(info)
	}
	if info, ok := mib.ClauseDescription(name); ok {
		return clauseHover(info)
	}

	sym := m.Symbol(name)
	switch {
	case sym.Object() != nil:
		return objectHover(sym.Object())
	case sym.Notification() != nil:
		return notificationHover(sym.Notification())
	case sym.Group() != nil:
		return groupHover(sym.Group())
	case sym.Compliance() != nil:
		return complianceHover(sym.Compliance())
	case sym.Capability() != nil:
		return capabilityHover(sym.Capability())
	case sym.Type() != nil:
		return typeHover(sym.Type())
	case sym.Node() != nil:
		return nodeHover(sym.Node())
	}

	// Module names aren't covered by Symbol lookup.
	if mod := m.Module(name); mod != nil {
		return moduleHover(mod)
	}

	return ""
}

func macroHover(info mib.MacroInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** (macro)\n\n", info.Name)
	fmt.Fprintf(&b, "Defined in %s (%s)\n\n", info.Module, info.RFC)
	b.WriteString(info.Description)
	b.WriteString("\n")
	return b.String()
}

func clauseHover(info mib.ClauseInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** (clause)\n\n", info.Name)
	fmt.Fprintf(&b, "Used in %s\n\n", strings.Join(info.Macros, ", "))
	b.WriteString(info.Description)
	b.WriteString("\n")
	return b.String()
}

func objectHover(obj *mib.Object) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s** (OBJECT-TYPE)\n\n", obj.Name())
	writeOIDLine(&b, obj.Node())
	b.WriteString("\n")

	if t := obj.Type(); t != nil {
		writeSyntax(&b, t)
	}
	if obj.Access() != 0 {
		fmt.Fprintf(&b, "MAX-ACCESS: %s\n\n", obj.Access())
	}
	if obj.Status() != 0 {
		fmt.Fprintf(&b, "STATUS: %s\n\n", obj.Status())
	}
	writeDescription(&b, obj.Description())
	return b.String()
}

func notificationHover(notif *mib.Notification) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s** (NOTIFICATION-TYPE)\n\n", notif.Name())
	writeOIDLine(&b, notif.Node())
	b.WriteString("\n")

	if objs := notif.Objects(); len(objs) > 0 {
		names := make([]string, len(objs))
		for i, o := range objs {
			names[i] = o.Name()
		}
		fmt.Fprintf(&b, "OBJECTS: %s\n\n", strings.Join(names, ", "))
	}
	if notif.Status() != 0 {
		fmt.Fprintf(&b, "STATUS: %s\n\n", notif.Status())
	}
	writeDescription(&b, notif.Description())
	return b.String()
}

func groupHover(grp *mib.Group) string {
	var b strings.Builder

	kind := "OBJECT-GROUP"
	if grp.IsNotificationGroup() {
		kind = "NOTIFICATION-GROUP"
	}
	fmt.Fprintf(&b, "**%s** (%s)\n\n", grp.Name(), kind)
	writeOIDLine(&b, grp.Node())
	b.WriteString("\n")

	if members := grp.Members(); len(members) > 0 {
		names := make([]string, len(members))
		for i, m := range members {
			names[i] = m.Name()
		}
		fmt.Fprintf(&b, "OBJECTS: %s\n\n", strings.Join(names, ", "))
	}
	if grp.Status() != 0 {
		fmt.Fprintf(&b, "STATUS: %s\n\n", grp.Status())
	}
	writeDescription(&b, grp.Description())
	return b.String()
}

func complianceHover(comp *mib.Compliance) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s** (MODULE-COMPLIANCE)\n\n", comp.Name())
	writeOIDLine(&b, comp.Node())
	b.WriteString("\n")

	if comp.Status() != 0 {
		fmt.Fprintf(&b, "STATUS: %s\n\n", comp.Status())
	}
	writeDescription(&b, comp.Description())
	return b.String()
}

func capabilityHover(c *mib.Capability) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s** (AGENT-CAPABILITIES)\n\n", c.Name())
	writeOIDLine(&b, c.Node())
	b.WriteString("\n")

	if rel := c.ProductRelease(); rel != "" {
		fmt.Fprintf(&b, "PRODUCT-RELEASE: %q\n\n", rel)
	}
	if c.Status() != 0 {
		fmt.Fprintf(&b, "STATUS: %s\n\n", c.Status())
	}
	writeDescription(&b, c.Description())
	return b.String()
}

func nodeHover(nd *mib.Node) string {
	var b strings.Builder

	name := nd.Name()
	if name == "" {
		return ""
	}

	fmt.Fprintf(&b, "**%s**\n\n", name)
	writeOIDLine(&b, nd)
	b.WriteString("\n")
	writeDescription(&b, nd.Description())
	return b.String()
}

func typeHover(t *mib.Type) string {
	var b strings.Builder

	kind := "type"
	if t.IsTextualConvention() {
		kind = "TEXTUAL-CONVENTION"
	}
	fmt.Fprintf(&b, "**%s** (%s)\n\n", t.Name(), kind)

	if mod := t.Module(); mod != nil {
		fmt.Fprintf(&b, "*%s*\n\n", mod.Name())
	}

	if base := t.EffectiveBase(); base != 0 {
		s := base.String()
		if hint := t.EffectiveDisplayHint(); hint != "" {
			s += fmt.Sprintf(" (DISPLAY-HINT %q)", hint)
		}
		fmt.Fprintf(&b, "Base: %s\n\n", s)
	}

	writeConstraints(&b, t)

	if t.Status() != 0 {
		fmt.Fprintf(&b, "STATUS: %s\n\n", t.Status())
	}
	writeDescription(&b, t.Description())
	return b.String()
}

func moduleHover(mod *mib.Module) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**%s** (module)\n\n", mod.Name())
	if oid := mod.OID(); len(oid) > 0 {
		fmt.Fprintf(&b, "`%s`\n\n", oid)
	}
	if org := mod.Organization(); org != "" {
		fmt.Fprintf(&b, "ORGANIZATION: %s\n\n", truncate(org, 200))
	}
	writeDescription(&b, mod.Description())
	return b.String()
}

// writeOIDLine writes the OID and module name for a node.
func writeOIDLine(b *strings.Builder, nd *mib.Node) {
	if nd == nil {
		return
	}
	if oid := nd.OID(); len(oid) > 0 {
		fmt.Fprintf(b, "`%s`", oid)
	}
	if mod := nd.Module(); mod != nil {
		fmt.Fprintf(b, " - *%s*", mod.Name())
	}
}

// writeSyntax writes the SYNTAX line for an object.
func writeSyntax(b *strings.Builder, t *mib.Type) {
	name := t.Name()
	if name == "" {
		name = t.EffectiveBase().String()
	}

	s := name
	if sizes := t.EffectiveSizes(); len(sizes) > 0 {
		s += " (SIZE " + formatRangeList(sizes) + ")"
	} else if ranges := t.EffectiveRanges(); len(ranges) > 0 {
		s += " (" + formatRangeList(ranges) + ")"
	}

	fmt.Fprintf(b, "SYNTAX: %s\n\n", s)

	if enums := t.EffectiveEnums(); len(enums) > 0 {
		writeEnumList(b, enums)
	}
}

// writeConstraints writes SIZE/range constraints for a type definition.
func writeConstraints(b *strings.Builder, t *mib.Type) {
	if sizes := t.EffectiveSizes(); len(sizes) > 0 {
		fmt.Fprintf(b, "SIZE: %s\n\n", formatRangeList(sizes))
	}
	if ranges := t.EffectiveRanges(); len(ranges) > 0 {
		fmt.Fprintf(b, "RANGE: %s\n\n", formatRangeList(ranges))
	}
	if enums := t.EffectiveEnums(); len(enums) > 0 {
		writeEnumList(b, enums)
	}
}

func writeEnumList(b *strings.Builder, enums []mib.NamedValue) {
	b.WriteString("Values: ")
	for i, e := range enums {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "%s(%d)", e.Label, e.Value)
		if i >= 15 {
			fmt.Fprintf(b, ", ... (%d total)", len(enums))
			break
		}
	}
	b.WriteString("\n\n")
}

func writeDescription(b *strings.Builder, desc string) {
	if desc == "" {
		return
	}
	b.WriteString("---\n\n")
	b.WriteString(truncate(strings.TrimSpace(desc), 1000))
	b.WriteString("\n")
}

func formatRangeList(ranges []mib.Range) string {
	parts := make([]string, len(ranges))
	for i, r := range ranges {
		parts[i] = r.String()
	}
	return strings.Join(parts, " | ")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
