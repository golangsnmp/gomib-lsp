package handler

import (
	"path/filepath"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// mibExtensions is the set of file extensions considered MIB files for the
// purpose of triggering reloads. Matches gomib.DefaultExtensions minus the
// empty-string (extension-less) case, which cannot be expressed as a glob.
var mibExtensions = map[string]struct{}{
	".mib": {},
	".smi": {},
	".txt": {},
	".my":  {},
}

// isMIBPath returns true if p has an extension gomib recognizes as a MIB file.
func isMIBPath(p string) bool {
	_, ok := mibExtensions[strings.ToLower(filepath.Ext(p))]
	return ok
}

// workspaceDidChangeWorkspaceFolders updates the server's root set when the
// client adds or removes workspace folders, then triggers a reload.
func (s *Server) workspaceDidChangeWorkspaceFolders(ctx *glsp.Context, params *protocol.DidChangeWorkspaceFoldersParams) error {
	s.mu.Lock()
	// Remove dropped folders.
	for _, removed := range params.Event.Removed {
		p := uriToPath(removed.URI)
		if p == "" {
			continue
		}
		for i, root := range s.workspaceRoots {
			if root == p {
				s.workspaceRoots = append(s.workspaceRoots[:i], s.workspaceRoots[i+1:]...)
				break
			}
		}
	}
	// Add new folders.
	for _, added := range params.Event.Added {
		p := uriToPath(added.URI)
		if p == "" {
			continue
		}
		s.workspaceRoots = append(s.workspaceRoots, p)
	}
	s.mu.Unlock()

	s.requestReload("workspace-folders")
	return nil
}

// workspaceDidChangeWatchedFiles triggers a reload when watched MIB files
// change on disk. Events for non-MIB files are ignored defensively; the
// registered glob should already filter them.
func (s *Server) workspaceDidChangeWatchedFiles(ctx *glsp.Context, params *protocol.DidChangeWatchedFilesParams) error {
	for _, change := range params.Changes {
		p := uriToPath(change.URI)
		if p != "" && isMIBPath(p) {
			s.requestReload("watched-files")
			return nil
		}
	}
	return nil
}

// workspaceDidCreateFiles triggers a reload when MIB files are created via
// an editor operation (new file, etc).
func (s *Server) workspaceDidCreateFiles(ctx *glsp.Context, params *protocol.CreateFilesParams) error {
	for _, f := range params.Files {
		p := uriToPath(f.URI)
		if p != "" && isMIBPath(p) {
			s.requestReload("did-create-files")
			return nil
		}
	}
	return nil
}

// workspaceDidRenameFiles triggers a reload when MIB files are renamed via
// an editor operation.
func (s *Server) workspaceDidRenameFiles(ctx *glsp.Context, params *protocol.RenameFilesParams) error {
	for _, f := range params.Files {
		pOld := uriToPath(f.OldURI)
		pNew := uriToPath(f.NewURI)
		if (pOld != "" && isMIBPath(pOld)) || (pNew != "" && isMIBPath(pNew)) {
			s.requestReload("did-rename-files")
			return nil
		}
	}
	return nil
}

// workspaceDidDeleteFiles triggers a reload when MIB files are deleted via
// an editor operation.
func (s *Server) workspaceDidDeleteFiles(ctx *glsp.Context, params *protocol.DeleteFilesParams) error {
	for _, f := range params.Files {
		p := uriToPath(f.URI)
		if p != "" && isMIBPath(p) {
			s.requestReload("did-delete-files")
			return nil
		}
	}
	return nil
}

// registerFileWatchers sends client/registerCapability for
// workspace/didChangeWatchedFiles. Non-fatal: if the client rejects the
// call or does not support dynamic registration, the server logs and
// continues. Other reload paths still work.
func (s *Server) registerFileWatchers(ctx *glsp.Context) {
	if ctx == nil || ctx.Call == nil {
		return
	}
	params := protocol.RegistrationParams{
		Registrations: []protocol.Registration{{
			ID:     "gomib-lsp/watched-files",
			Method: protocol.MethodWorkspaceDidChangeWatchedFiles,
			RegisterOptions: protocol.DidChangeWatchedFilesRegistrationOptions{
				Watchers: []protocol.FileSystemWatcher{
					{GlobPattern: "**/*.{mib,smi,txt,my}"},
				},
			},
		}},
	}
	ctx.Call(protocol.ServerClientRegisterCapability, params, nil)
}
