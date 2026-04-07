package lsp

import "runtime/debug"

// serverVersion returns the gomib-lsp version from the Go build info.
// Returns the module version when installed, "dev+<shortsha>" when built
// from a VCS checkout, or "dev" as a final fallback.
func serverVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return "dev+" + s.Value[:7]
		}
	}
	return "dev"
}
