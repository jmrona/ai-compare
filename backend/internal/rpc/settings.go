package rpc

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"connectrpc.com/connect/v2"

	"ai-compare/backend/internal/comparison"
	v1 "ai-compare/backend/internal/gen/aicompare/v1"
	"ai-compare/backend/internal/settings"
	"ai-compare/backend/internal/workspace"
)

// Environment facts the settings page shows but cannot change.
type Env struct {
	OpenAIKey    bool
	AnthropicKey bool
	LocalBaseURL string
}

type settingsService struct {
	settings    *settings.Service
	comparisons *comparison.Service
	ws          *workspace.Service
	env         Env

	mu         sync.Mutex
	latest     string
	latestRead time.Time
}

func (s *settingsService) GetSettings(ctx context.Context, _ *v1.GetSettingsRequest) (*v1.GetSettingsResponse, error) {
	return &v1.GetSettingsResponse{Settings: s.toProto(ctx, s.settings.Get())}, nil
}

func (s *settingsService) UpdateSettings(ctx context.Context, req *v1.UpdateSettingsRequest) (*v1.UpdateSettingsResponse, error) {
	in := req.GetSettings()
	l := in.GetDefaultLimits()
	n := settings.Settings{
		DefaultLimits: settings.Limits{TimeoutMin: l.TimeoutMin, MaxTokensK: l.MaxTokensK, MaxCostUSD: l.MaxCostUsd},
		ReportModel:   in.GetReportModel(),
		AutoReport:    in.GetAutoReport(),
		CPUs:          in.GetResources().GetCpus(),
		MemoryGB:      in.GetResources().GetMemoryGb(),
		RetentionDays: int(in.GetRetentionDays()),
		Retention:     s.settings.Get().Retention,
	}
	if r := in.GetRetention(); r != nil {
		n.Retention = settings.Retention{Containers: r.GetContainers(), Images: r.GetImages(), Staging: r.GetProjectCopies(), Artifacts: r.GetArtefacts()}
	}
	saved, err := s.settings.Update(ctx, n)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err.Error())
	}
	return &v1.UpdateSettingsResponse{Settings: s.toProto(ctx, saved)}, nil
}

func (s *settingsService) CleanUp(ctx context.Context, _ *v1.CleanUpRequest) (*v1.CleanUpResponse, error) {
	if s.comparisons == nil {
		return nil, connect.NewError(connect.CodeUnavailable, "Docker is not reachable")
	}
	st := s.settings.Get()
	n, removed, err := s.comparisons.CleanUp(ctx, time.Duration(st.RetentionDays)*24*time.Hour, st.Retention)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err.Error())
	}
	return &v1.CleanUpResponse{Comparisons: int32(n), Containers: int32(removed.Containers), Images: int32(removed.Images), ProjectCopies: int32(removed.Staging), Artefacts: int32(removed.Artifacts)}, nil
}

func (s *settingsService) toProto(ctx context.Context, st settings.Settings) *v1.Settings {
	out := &v1.Settings{
		Keys:            &v1.ProviderKeys{Openai: s.env.OpenAIKey, Anthropic: s.env.AnthropicKey},
		DefaultLimits:   &v1.Limits{TimeoutMin: st.DefaultLimits.TimeoutMin, MaxTokensK: st.DefaultLimits.MaxTokensK, MaxCostUsd: st.DefaultLimits.MaxCostUSD},
		SuggestedLimits: &v1.Limits{TimeoutMin: settings.Suggested.TimeoutMin, MaxTokensK: settings.Suggested.MaxTokensK, MaxCostUsd: settings.Suggested.MaxCostUSD},
		ReportModel:     st.ReportModel,
		AutoReport:      st.AutoReport,
		Resources:       &v1.Resources{Cpus: st.CPUs, MemoryGb: st.MemoryGB},
		LocalBaseUrl:    s.env.LocalBaseURL,
		CliVersions: []*v1.CliVersion{
			{Cli: "opencode", Pinned: comparison.OpencodeVersion, Latest: s.latestOpencode(ctx)},
			{Cli: "codex"},
			{Cli: "claude"},
		},
		RetentionDays: int32(st.RetentionDays),
		Retention:     &v1.Retention{Containers: st.Retention.Containers, Images: st.Retention.Images, ProjectCopies: st.Retention.Staging, Artefacts: st.Retention.Artifacts},
	}
	if s.ws != nil {
		d := s.ws.DiskUsage(ctx)
		out.Disk = []*v1.DiskUsage{
			{Label: "images", Bytes: d.Images},
			{Label: "artefacts", Bytes: d.Artifacts},
			{Label: "project copies", Bytes: d.Staging},
		}
	}
	return out
}

// latestOpencode reads the latest opencode version from npm, at most once an hour.
func (s *settingsService) latestOpencode(ctx context.Context) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Since(s.latestRead) < time.Hour {
		return s.latest
	}
	s.latestRead = time.Now()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://registry.npmjs.org/opencode-ai/latest", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return s.latest
	}
	defer res.Body.Close()
	var v struct {
		Version string `json:"version"`
	}
	if json.NewDecoder(res.Body).Decode(&v) == nil && v.Version != "" {
		s.latest = v.Version
	}
	return s.latest
}
