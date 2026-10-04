package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"connectrpc.com/connect/v2"

	"ai-compare/backend/internal/comparison"
	v1 "ai-compare/backend/internal/gen/aicompare/v1"
	"ai-compare/backend/internal/report"
)

type reportService struct {
	svc         *report.Service
	comparisons *comparison.Service
}

type userVerdict struct {
	Verdict string `json:"verdict"`
	Note    string `json:"note"`
}

func (s *reportService) GenerateReport(ctx context.Context, req *v1.GenerateReportRequest) (*v1.GenerateReportResponse, error) {
	if err := s.svc.Generate(ctx, req.GetComparisonId()); err != nil {
		if errors.Is(err, comparison.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, err.Error())
		}
		return nil, connect.NewError(connect.CodeFailedPrecondition, err.Error())
	}
	return &v1.GenerateReportResponse{}, nil
}

func (s *reportService) GenerateCriteria(ctx context.Context, req *v1.GenerateCriteriaRequest) (*v1.GenerateCriteriaResponse, error) {
	if strings.TrimSpace(req.GetPrompt()) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, "write the prompt first")
	}
	crit, err := s.svc.GenerateCriteria(ctx, req.GetPrompt())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err.Error())
	}
	return &v1.GenerateCriteriaResponse{Criteria: CriteriaToProto(crit)}, nil
}

func (s *reportService) SetUserVerdict(ctx context.Context, req *v1.SetUserVerdictRequest) (*v1.SetUserVerdictResponse, error) {
	var data []byte
	switch req.GetVerdict() {
	case "":
	case "agree", "other", "tie":
		data, _ = json.Marshal(userVerdict{Verdict: req.GetVerdict(), Note: strings.TrimSpace(req.GetNote())})
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, "the verdict must be agree, other or tie")
	}
	if err := s.comparisons.SetUserVerdict(ctx, req.GetComparisonId(), data); err != nil {
		return nil, notFound(err)
	}
	return &v1.SetUserVerdictResponse{}, nil
}

func (s *reportService) ExportReport(_ context.Context, req *v1.ExportReportRequest) (*v1.ExportReportResponse, error) {
	v, err := s.comparisons.Get(req.GetComparisonId())
	if err != nil {
		return nil, notFound(err)
	}
	r, err := s.svc.Get(v.ID)
	if err != nil || r == nil || r.Status != "ready" || r.Version < report.Version {
		return nil, connect.NewError(connect.CodeFailedPrecondition, "there is no report in the current format to export; generate it first")
	}
	return &v1.ExportReportResponse{Filename: "ai-compare-" + v.ID + "-report.md", Markdown: report.Markdown(v, r)}, nil
}

func (s *reportService) GetReport(_ context.Context, req *v1.GetReportRequest) (*v1.GetReportResponse, error) {
	r, err := s.svc.Get(req.GetComparisonId())
	if err != nil {
		return nil, notFound(err)
	}
	if r == nil {
		return &v1.GetReportResponse{}, nil
	}
	out := &v1.Report{
		Version: int32(r.Version), ComparisonId: r.ComparisonID, Status: r.Status, Error: r.Error, Model: r.Model, JudgeModel: r.JudgeModel,
		CostUsd: r.CostUSD, Headline: r.Headline, Criteria: CriteriaToProto(r.Criteria), CriteriaBy: r.CriteriaBy, Warnings: r.Warnings,
	}
	if v, err := s.comparisons.Get(r.ComparisonID); err == nil && v.UserVerdict != nil {
		var uv userVerdict
		if json.Unmarshal(v.UserVerdict, &uv) == nil {
			out.UserVerdict = &v1.UserVerdict{Verdict: uv.Verdict, Note: uv.Note}
		}
	}
	if r.Sides != nil {
		out.A, out.B = sideReportToProto(r.Sides["A"]), sideReportToProto(r.Sides["B"])
	}
	if j := r.Judge; j != nil {
		out.Judge = &v1.Judgement{Winner: j.Winner, Confidence: j.Confidence, Reasons: j.Reasons, Disagreements: j.Disagreements,
			PassesAgree: j.PassesAgree, Passes: j.Passes, Ship: map[string]*v1.Ship{}}
		for k, sh := range j.Ship {
			out.Judge.Ship[k] = &v1.Ship{Yes: sh.Yes, Reason: sh.Reason}
		}
		for _, l := range j.Labels {
			out.Judge.Labels = append(out.Judge.Labels, &v1.Verdict{Label: l.Label, Side: l.Side})
		}
	}
	return &v1.GetReportResponse{Report: out}, nil
}

func sideReportToProto(s *report.SideReport) *v1.SideReport {
	if s == nil {
		return nil
	}
	out := &v1.SideReport{Analysis: s.Analysis, NotVerified: s.NotVerified,
		Review: &v1.Review{NotReviewed: s.Review.NotReviewed}, Score: &v1.Score{Total: s.Score.Total}}
	for _, g := range s.Gates {
		out.Gates = append(out.Gates, &v1.Gate{Key: g.Key, Label: g.Label, Passed: g.Passed, Reason: g.Reason})
	}
	for _, c := range s.Criteria {
		out.Criteria = append(out.Criteria, &v1.CriterionCheck{Index: int32(c.Index), Status: c.Status, Method: c.Method, Evidence: c.Evidence})
	}
	for _, f := range s.Review.Problems {
		out.Review.Problems = append(out.Review.Problems, &v1.Finding{Severity: f.Severity, Title: f.Title, Impact: f.Impact, Location: f.Location})
	}
	for _, st := range s.Review.Strengths {
		out.Review.Strengths = append(out.Review.Strengths, &v1.Strength{Title: st.Title, Location: st.Location})
	}
	for _, p := range s.Score.Parts {
		sp := &v1.ScorePart{Key: p.Key, Label: p.Label, Points: p.Points, Max: p.Max}
		for _, l := range p.Lines {
			sp.Lines = append(sp.Lines, &v1.ScoreLine{Label: l.Label, Points: l.Points, Max: l.Max, Detail: l.Detail})
		}
		out.Score.Parts = append(out.Score.Parts, sp)
	}
	if h := s.Harness; h != nil {
		out.Harness = &v1.HarnessCost{FirstRequestTokens: h.FirstRequestTokens, Parts: costParts(h.Parts), PerRequest: h.PerRequest,
			Requests: int32(h.Requests), Total: h.Total, CacheShare: h.CacheShare, CostUsd: h.CostUSD, ShareOfSide: h.ShareOfSide,
			Files: costParts(h.Files), Skills: costParts(h.Skills), SkillsLoaded: costParts(h.SkillsLoaded)}
	}
	if a := s.Audit; a != nil {
		out.Audit = &v1.HarnessAudit{Strengths: auditItems(a.Strengths), Gaps: auditItems(a.Gaps)}
		for _, sg := range a.Suggestions {
			out.Audit.Suggestions = append(out.Audit.Suggestions, &v1.Suggestion{Kind: sg.Kind, File: sg.File, Change: sg.Change, Evidence: sg.Evidence, TokensSaved: sg.TokensSaved})
		}
	}
	for _, a := range s.Subagents {
		out.Subagents = append(out.Subagents, &v1.Subagent{Type: a.Type, Description: a.Description, Model: a.Model, Status: a.Status,
			DurationSec: a.DurationSec, Tokens: a.Tokens, CostUsd: a.CostUSD, Tools: counts(a.Tools)})
	}
	ss := s.Session
	out.Session = &v1.SessionSummary{Requests: int32(ss.Requests), CacheShare: ss.CacheShare, ReasoningSteps: int32(ss.ReasoningSteps),
		ReasoningTokens: ss.ReasoningTokens, FirstEditSec: ss.FirstEditSec, Tools: counts(ss.Tools), ToolCalls: int32(ss.ToolCalls),
		ToolFailures: int32(ss.ToolFailures), EndsWithQuestion: ss.EndsWithQuestion, LongContextRequests: int32(ss.LongContextRequests),
		ProviderErrors: int32(ss.ProviderErrors), RateLimited: int32(ss.RateLimited), ReasoningPoints: ss.ReasoningPoints}
	for _, c := range ss.FailedCommands {
		out.Session.FailedCommands = append(out.Session.FailedCommands, &v1.FailedCommand{Command: c.Command, ExitCode: int32(c.ExitCode), Fixed: c.Fixed, Agent: c.Agent})
	}
	for _, p := range ss.RequestPoints {
		out.Session.RequestPoints = append(out.Session.RequestPoints, &v1.RequestPoint{AtSec: p.AtSec, Context: p.Context, CostUsd: p.CostUSD})
	}
	return out
}

func costParts(in []report.CostPart) []*v1.CostPart {
	out := make([]*v1.CostPart, 0, len(in))
	for _, p := range in {
		out = append(out, &v1.CostPart{Label: p.Label, Tokens: p.Tokens})
	}
	return out
}

func auditItems(in []report.AuditItem) []*v1.AuditItem {
	out := make([]*v1.AuditItem, 0, len(in))
	for _, it := range in {
		out = append(out, &v1.AuditItem{Title: it.Title, Evidence: it.Evidence})
	}
	return out
}

func counts(in map[string]int) map[string]int32 {
	out := make(map[string]int32, len(in))
	for k, v := range in {
		out[k] = int32(v)
	}
	return out
}
