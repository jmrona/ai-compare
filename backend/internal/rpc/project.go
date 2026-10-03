package rpc

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"connectrpc.com/connect/v2"

	v1 "ai-compare/backend/internal/gen/aicompare/v1"
	"ai-compare/backend/internal/workspace"
)

type projectService struct {
	ws *workspace.Service
	// home is the user's home folder on the host, where the folder browser starts.
	home string
}

func (s *projectService) InspectProject(ctx context.Context, req *v1.InspectProjectRequest) (*v1.InspectProjectResponse, error) {
	path := strings.TrimSpace(req.GetPath())
	if path == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, "send the absolute path of the project")
	}
	ins, err := s.ws.InspectProject(ctx, path)
	if err != nil {
		return nil, workspaceError(err)
	}
	return &v1.InspectProjectResponse{Inspection: inspectionToProto(path, ins)}, nil
}

func (s *projectService) ListFolders(ctx context.Context, req *v1.ListFoldersRequest) (*v1.ListFoldersResponse, error) {
	path := strings.TrimSpace(req.GetPath())
	if path == "" {
		path = s.home
	}
	if path == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "the home folder is unknown (HOST_HOME is not set); type a path instead")
	}
	l, err := s.ws.ListFolders(ctx, path)
	if err != nil {
		return nil, workspaceError(err)
	}
	out := &v1.ListFoldersResponse{Path: l.Path, Parent: l.Parent}
	for _, f := range l.Folders {
		out.Folders = append(out.Folders, &v1.Folder{Name: f.Name, Path: f.Path, IsGit: f.IsGit})
	}
	return out, nil
}

// workspaceError tells the user's mistakes (a wrong or unshared path) from failures of ai-compare.
func workspaceError(err error) error {
	if errors.Is(err, workspace.ErrPathNotFound) || errors.Is(err, workspace.ErrPathNotShared) || strings.Contains(err.Error(), "must be absolute") || strings.Contains(err.Error(), "root folder") {
		return connect.NewError(connect.CodeInvalidArgument, err.Error())
	}
	return connect.NewError(connect.CodeInternal, err.Error())
}

var harnessDirs = map[string]bool{".claude": true, ".agents": true, ".codex": true, ".opencode": true, ".cursor": true}

// Which CLI reads each harness file at the project root.
var harnessReaders = map[string][]string{
	"AGENTS.md":       {"opencode", "codex"},
	"CLAUDE.md":       {"claude"},
	"CLAUDE.local.md": {"claude"},
	"GEMINI.md":       {},
	".claude":         {"claude"},
	".agents":         {"codex"},
	".codex":          {"codex"},
	".opencode":       {"opencode"},
	"opencode.json":   {"opencode"},
	"opencode.jsonc":  {"opencode"},
	".mcp.json":       {"claude"},
	".cursor":         {},
	".cursorrules":    {},
}

func inspectionToProto(path string, ins workspace.Inspection) *v1.ProjectInspection {
	out := &v1.ProjectInspection{
		Path:      path,
		Name:      filepath.Base(strings.ReplaceAll(path, `\`, "/")),
		IsGit:     ins.Git,
		FileCount: int64(ins.Files),
		SizeBytes: ins.Bytes,
		Excluded:  ins.EnvFiles,
		Profile:   detectProfile(ins.Markers),
	}
	for _, h := range ins.Harness {
		name := h
		if harnessDirs[h] {
			name += "/"
		}
		out.HarnessFiles = append(out.HarnessFiles, &v1.HarnessFile{Path: name, ReadBy: harnessReaders[h]})
	}
	return out
}

// detectProfile proposes the runtime and commands from the files at the project root.
// The runtime must have Node.js for now, because opencode is installed with npm.
func detectProfile(markers []string) *v1.ProjectProfile {
	p := &v1.ProjectProfile{Runtime: "node:22-bookworm-slim"}
	has := func(m string) bool { return slices.Contains(markers, m) }
	switch {
	case has("pnpm-lock.yaml"):
		p.Setup = "corepack enable && pnpm install --frozen-lockfile"
		p.Test = "pnpm test"
	case has("yarn.lock"):
		p.Setup = "corepack enable && yarn install --frozen-lockfile"
		p.Test = "yarn test"
	case has("package-lock.json"):
		p.Setup = "npm ci"
		p.Test = "npm test"
	case has("package.json"):
		p.Setup = "npm install"
		p.Test = "npm test"
	}
	return p
}
