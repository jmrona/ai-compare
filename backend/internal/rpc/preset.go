package rpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"connectrpc.com/connect/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	"ai-compare/backend/internal/comparison"
	v1 "ai-compare/backend/internal/gen/aicompare/v1"
	"ai-compare/backend/internal/presets"
	"ai-compare/backend/internal/workspace"
)

type presetService struct {
	store       *presets.Store
	comparisons *comparison.Service
	ws          *workspace.Service
}

func (s *presetService) toProto(p presets.Preset) *v1.Preset {
	out := &v1.Preset{
		Slug: p.Slug, Title: p.Title, Description: p.Description, Clis: p.CLIs, Notes: p.Notes, Hash: p.Hash,
		UpdatedAt: timestamppb.New(p.UpdatedAt),
	}
	if s.comparisons != nil {
		out.Uses = int32(s.comparisons.PresetUses(p.Slug))
	}
	for _, f := range p.Files {
		out.Files = append(out.Files, &v1.PresetFile{Root: f.Root, Path: f.Path, Size: f.Size, Category: f.Category})
	}
	return out
}

// get returns the preset after a change, as the response of most methods.
func (s *presetService) get(slug string) (*v1.Preset, error) {
	p, err := s.store.Get(slug)
	if err != nil {
		return nil, presetError(err)
	}
	return s.toProto(p), nil
}

func presetError(err error) error {
	if errors.Is(err, presets.ErrNotFound) {
		return connect.NewError(connect.CodeNotFound, err.Error())
	}
	return connect.NewError(connect.CodeInvalidArgument, err.Error())
}

func (s *presetService) ListPresets(context.Context, *v1.ListPresetsRequest) (*v1.ListPresetsResponse, error) {
	list, err := s.store.List()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err.Error())
	}
	out := &v1.ListPresetsResponse{}
	for _, p := range list {
		out.Presets = append(out.Presets, s.toProto(p))
	}
	return out, nil
}

func (s *presetService) GetPreset(_ context.Context, req *v1.GetPresetRequest) (*v1.GetPresetResponse, error) {
	p, err := s.get(req.GetSlug())
	if err != nil {
		return nil, err
	}
	return &v1.GetPresetResponse{Preset: p}, nil
}

func (s *presetService) CreatePreset(_ context.Context, req *v1.CreatePresetRequest) (*v1.CreatePresetResponse, error) {
	p, err := s.store.Create(req.GetTitle(), req.GetDescription(), req.GetClis())
	if err != nil {
		return nil, presetError(err)
	}
	return &v1.CreatePresetResponse{Preset: s.toProto(p)}, nil
}

func (s *presetService) UpdatePreset(_ context.Context, req *v1.UpdatePresetRequest) (*v1.UpdatePresetResponse, error) {
	p, err := s.store.Update(req.GetSlug(), req.GetTitle(), req.GetDescription(), req.GetClis(), req.GetNotes())
	if err != nil {
		return nil, presetError(err)
	}
	return &v1.UpdatePresetResponse{Preset: s.toProto(p)}, nil
}

func (s *presetService) DuplicatePreset(_ context.Context, req *v1.DuplicatePresetRequest) (*v1.DuplicatePresetResponse, error) {
	p, err := s.store.Duplicate(req.GetSlug(), req.GetTitle())
	if err != nil {
		return nil, presetError(err)
	}
	return &v1.DuplicatePresetResponse{Preset: s.toProto(p)}, nil
}

func (s *presetService) DeletePreset(_ context.Context, req *v1.DeletePresetRequest) (*v1.DeletePresetResponse, error) {
	if err := s.store.Delete(req.GetSlug()); err != nil {
		return nil, presetError(err)
	}
	return &v1.DeletePresetResponse{}, nil
}

func (s *presetService) GetPresetFile(_ context.Context, req *v1.GetPresetFileRequest) (*v1.GetPresetFileResponse, error) {
	data, err := s.store.ReadFile(req.GetSlug(), req.GetRoot(), req.GetPath())
	if err != nil {
		return nil, presetError(err)
	}
	return &v1.GetPresetFileResponse{Content: data}, nil
}

func (s *presetService) WritePresetFile(_ context.Context, req *v1.WritePresetFileRequest) (*v1.WritePresetFileResponse, error) {
	warnings, err := s.store.WriteFile(req.GetSlug(), req.GetRoot(), req.GetPath(), req.GetContent())
	if err != nil {
		return nil, presetError(err)
	}
	p, err := s.get(req.GetSlug())
	if err != nil {
		return nil, err
	}
	return &v1.WritePresetFileResponse{Preset: p, Warnings: warnings}, nil
}

func (s *presetService) DeletePresetFile(_ context.Context, req *v1.DeletePresetFileRequest) (*v1.DeletePresetFileResponse, error) {
	if err := s.store.DeleteFile(req.GetSlug(), req.GetRoot(), req.GetPath()); err != nil {
		return nil, presetError(err)
	}
	p, err := s.get(req.GetSlug())
	if err != nil {
		return nil, err
	}
	return &v1.DeletePresetFileResponse{Preset: p}, nil
}

func (s *presetService) MovePresetFile(_ context.Context, req *v1.MovePresetFileRequest) (*v1.MovePresetFileResponse, error) {
	if err := s.store.MoveFile(req.GetSlug(), req.GetRoot(), req.GetPath(), req.GetNewRoot(), req.GetNewPath()); err != nil {
		return nil, presetError(err)
	}
	p, err := s.get(req.GetSlug())
	if err != nil {
		return nil, err
	}
	return &v1.MovePresetFileResponse{Preset: p}, nil
}

// ImportFromProject copies chosen harness files of a host project into the preset, through the
// read-only copy helper and a temporary folder in the staging volume.
func (s *presetService) ImportFromProject(ctx context.Context, req *v1.ImportFromProjectRequest) (*v1.ImportFromProjectResponse, error) {
	if s.ws == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "Docker is not reachable")
	}
	if _, err := s.store.Get(req.GetSlug()); err != nil {
		return nil, presetError(err)
	}
	b := make([]byte, 4)
	rand.Read(b)
	tmp := "import-" + hex.EncodeToString(b)
	defer s.ws.RemoveStaging(tmp)
	dir, err := s.ws.ImportPaths(ctx, strings.TrimSpace(req.GetProjectPath()), tmp, req.GetPaths())
	if err != nil {
		return nil, workspaceError(err)
	}
	n, err := s.store.AddTree(req.GetSlug(), "project", dir)
	if err != nil {
		return nil, presetError(err)
	}
	p, err := s.get(req.GetSlug())
	if err != nil {
		return nil, err
	}
	return &v1.ImportFromProjectResponse{Preset: p, Files: int32(n)}, nil
}
