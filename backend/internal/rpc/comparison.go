package rpc

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	"ai-compare/backend/internal/comparison"
	v1 "ai-compare/backend/internal/gen/aicompare/v1"
)

type comparisonService struct {
	svc *comparison.Service
}

func (s *comparisonService) StartComparison(ctx context.Context, req *v1.StartComparisonRequest) (*v1.StartComparisonResponse, error) {
	p := req.GetProfile()
	id, err := s.svc.Start(ctx, comparison.NewComparison{
		ProjectPath: req.GetProjectPath(),
		Profile:     comparison.Profile{Runtime: p.GetRuntime(), Setup: p.GetSetup(), Test: p.GetTest(), HiddenTestsPath: p.GetHiddenTestsPath()},
		Prompt:      req.GetPrompt(),
		Sides:       map[string]comparison.SideConfig{"A": sideConfigFromProto(req.GetA()), "B": sideConfigFromProto(req.GetB())},
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err.Error())
	}
	return &v1.StartComparisonResponse{Id: id}, nil
}

func (s *comparisonService) GetComparison(_ context.Context, req *v1.GetComparisonRequest) (*v1.GetComparisonResponse, error) {
	v, err := s.svc.Get(req.GetId())
	if err != nil {
		return nil, notFound(err)
	}
	return &v1.GetComparisonResponse{Comparison: ComparisonToProto(v)}, nil
}

func (s *comparisonService) ListComparisons(context.Context, *v1.ListComparisonsRequest) (*v1.ListComparisonsResponse, error) {
	out := &v1.ListComparisonsResponse{}
	for _, v := range s.svc.List() {
		out.Comparisons = append(out.Comparisons, ComparisonToProto(v))
	}
	return out, nil
}

func (s *comparisonService) GetActiveComparison(context.Context, *v1.GetActiveComparisonRequest) (*v1.GetActiveComparisonResponse, error) {
	out := &v1.GetActiveComparisonResponse{}
	if v := s.svc.Active(); v != nil {
		out.Comparison = ComparisonToProto(*v)
	}
	return out, nil
}

func (s *comparisonService) FinishSide(_ context.Context, req *v1.FinishSideRequest) (*v1.FinishSideResponse, error) {
	if err := s.svc.Finish(req.GetId(), req.GetSide()); err != nil {
		return nil, notFound(err)
	}
	return &v1.FinishSideResponse{}, nil
}

func (s *comparisonService) CancelSide(_ context.Context, req *v1.CancelSideRequest) (*v1.CancelSideResponse, error) {
	if err := s.svc.Cancel(req.GetId(), req.GetSide()); err != nil {
		return nil, notFound(err)
	}
	return &v1.CancelSideResponse{}, nil
}

func (s *comparisonService) DeleteComparison(ctx context.Context, req *v1.DeleteComparisonRequest) (*v1.DeleteComparisonResponse, error) {
	if err := s.svc.Delete(ctx, req.GetId()); err != nil {
		if errors.Is(err, comparison.ErrNotFound) {
			return nil, notFound(err)
		}
		return nil, connect.NewError(connect.CodeFailedPrecondition, err.Error())
	}
	return &v1.DeleteComparisonResponse{}, nil
}

func (s *comparisonService) GetLogs(_ context.Context, req *v1.GetLogsRequest) (*v1.GetLogsResponse, error) {
	logs, err := s.svc.Logs(req.GetId(), req.GetSide())
	if err != nil {
		return nil, notFound(err)
	}
	out := &v1.GetLogsResponse{}
	for _, l := range logs {
		out.Entries = append(out.Entries, &v1.LogEntry{At: timestamppb.New(l.At), Level: l.Level, Source: l.Source, Message: l.Message})
	}
	return out, nil
}

func (s *comparisonService) GetDiff(ctx context.Context, req *v1.GetDiffRequest) (*v1.GetDiffResponse, error) {
	d, err := s.svc.Diff(ctx, req.GetId(), req.GetSide(), req.GetKind())
	if err != nil {
		if errors.Is(err, comparison.ErrNotFound) {
			return nil, notFound(err)
		}
		return nil, connect.NewError(connect.CodeUnavailable, err.Error())
	}
	out := &v1.GetDiffResponse{Files: filesToProto(d.Files), Truncated: d.Truncated, Ready: d.Ready}
	for _, l := range d.Lines {
		out.Lines = append(out.Lines, &v1.DiffLine{Kind: l.Kind, Text: l.Text})
	}
	return out, nil
}

func (s *comparisonService) GetTests(_ context.Context, req *v1.GetTestsRequest) (*v1.GetTestsResponse, error) {
	tests, visible, hidden, err := s.svc.TestOutput(req.GetId(), req.GetSide())
	if err != nil {
		return nil, notFound(err)
	}
	return &v1.GetTestsResponse{Tests: testsToProto(tests), VisibleOutput: visible, HiddenOutput: hidden}, nil
}

func (s *comparisonService) GetTimeline(_ context.Context, req *v1.GetTimelineRequest) (*v1.GetTimelineResponse, error) {
	tl, err := s.svc.Timeline(req.GetId(), req.GetSide())
	if err != nil {
		return nil, notFound(err)
	}
	out := &v1.GetTimelineResponse{Ready: tl.Ready, SessionCostUsd: tl.SessionCostUSD()}
	if u := tl.SessionUsage(); u != nil {
		out.SessionUsage = usageToProto(*u)
	}
	for _, e := range tl.Events {
		out.Events = append(out.Events, &v1.TimelineEvent{At: timestamppb.New(e.At), Kind: e.Kind, Detail: e.Detail})
	}
	return out, nil
}

func notFound(err error) error {
	if errors.Is(err, comparison.ErrNotFound) || strings.Contains(err.Error(), "does not exist") {
		return connect.NewError(connect.CodeNotFound, err.Error())
	}
	return connect.NewError(connect.CodeInternal, err.Error())
}

/* ── Conversions ──────────────────────────────────────────── */

func sideConfigFromProto(c *v1.SideConfig) comparison.SideConfig {
	l := c.GetLimits()
	return comparison.SideConfig{
		CLI: c.GetCli(), Provider: c.GetProvider(), Model: c.GetModel(), Effort: c.GetEffort(), Mode: c.GetMode(),
		Limits: comparison.Limits{TimeoutMin: l.TimeoutMin, MaxTokensK: l.MaxTokensK, MaxCostUSD: l.MaxCostUsd},
	}
}

func limitsToProto(l comparison.Limits) *v1.Limits {
	return &v1.Limits{TimeoutMin: l.TimeoutMin, MaxTokensK: l.MaxTokensK, MaxCostUsd: l.MaxCostUSD}
}

func usageToProto(u comparison.Usage) *v1.Usage {
	return &v1.Usage{Input: u.Input, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite, Output: u.Output}
}

func filesToProto(fs []comparison.FileChange) []*v1.FileChange {
	out := make([]*v1.FileChange, 0, len(fs))
	for _, f := range fs {
		out = append(out, &v1.FileChange{Path: f.Path, Added: int32(f.Added), Removed: int32(f.Removed)})
	}
	return out
}

func testRunToProto(t *comparison.TestRun) *v1.TestRun {
	if t == nil {
		return nil
	}
	return &v1.TestRun{Status: t.Status, ExitCode: int32(t.ExitCode), DurationSec: t.DurationSec}
}

func testsToProto(t comparison.Tests) *v1.Tests {
	return &v1.Tests{Command: t.Command, Visible: testRunToProto(t.Visible), Hidden: testRunToProto(t.Hidden), SkippedReason: t.SkippedReason}
}

func sideToProto(sv comparison.SideView) *v1.Side {
	m := sv.Metrics
	ps := &v1.PriceSnapshot{Price: priceToProto(sv.PriceSnapshot.Price)}
	if !sv.PriceSnapshot.FetchedAt.IsZero() {
		ps.FetchedAt = timestamppb.New(sv.PriceSnapshot.FetchedAt)
	}
	return &v1.Side{
		Key: sv.Key,
		Config: &v1.SideConfig{
			Cli: sv.Config.CLI, Provider: sv.Config.Provider, Model: sv.Config.Model, Effort: sv.Config.Effort, Mode: sv.Config.Mode,
			Limits: limitsToProto(sv.Config.Limits),
		},
		CliVersion: sv.CLIVersion, Status: sv.Status, EndReason: sv.EndReason, Failure: sv.Failure,
		Metrics: &v1.Metrics{
			ElapsedSec: m.ElapsedSec, AgentSec: m.AgentSec, HumanWaitSec: m.HumanWaitSec, PrepSec: m.PrepSec,
			Phases:  &v1.Phases{CopySec: m.Phases.CopySec, BuildSec: m.Phases.BuildSec, StartSec: m.Phases.StartSec, VerifySec: m.Phases.VerifySec},
			Usage:   usageToProto(m.Usage),
			CostUsd: m.CostUSD, CostConfirmedUsd: m.CostConfirmedUSD,
			Requests: int32(m.Requests), Retries: int32(m.Retries), Errors: int32(m.Errors), TokensPerSec: m.TokensPerSec,
		},
		Files: filesToProto(sv.Files), HarnessFiles: filesToProto(sv.HarnessFiles), Tests: testsToProto(sv.Tests),
		PriceSnapshot: ps, HasResult: sv.HasResult, HasRecording: sv.HasRecording,
	}
}

// ComparisonToProto converts a comparison view; the event stream uses it too.
func ComparisonToProto(v comparison.View) *v1.Comparison {
	return &v1.Comparison{
		Id: v.ID, CreatedAt: timestamppb.New(v.CreatedAt), ProjectPath: v.ProjectPath, ProjectName: v.ProjectName,
		Prompt: v.Prompt, Harness: v.Harness, Report: v.Report,
		Profile: &v1.ProjectProfile{Runtime: v.Profile.Runtime, Setup: v.Profile.Setup, Test: v.Profile.Test, HiddenTestsPath: v.Profile.HiddenTestsPath},
		A:       sideToProto(v.Sides["A"]), B: sideToProto(v.Sides["B"]),
	}
}
