package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect/v2"

	"ai-compare/backend/internal/comparison"
	v1 "ai-compare/backend/internal/gen/aicompare/v1"
	"ai-compare/backend/internal/report"
)

type reportService struct {
	svc *report.Service
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

func (s *reportService) GetReport(_ context.Context, req *v1.GetReportRequest) (*v1.GetReportResponse, error) {
	r, err := s.svc.Get(req.GetComparisonId())
	if err != nil {
		return nil, notFound(err)
	}
	if r == nil {
		return &v1.GetReportResponse{}, nil
	}
	out := &v1.Report{
		ComparisonId: r.ComparisonID, Status: r.Status, Error: r.Error, Model: r.Model, CostUsd: r.CostUSD,
		Conclusions: r.Conclusions, AnalysisA: r.Analysis["A"], AnalysisB: r.Analysis["B"], Warnings: r.Warnings,
	}
	for _, v := range r.Verdicts {
		out.Verdicts = append(out.Verdicts, &v1.Verdict{Label: v.Label, Side: v.Side})
	}
	for _, f := range r.Findings {
		out.Findings = append(out.Findings, &v1.Finding{Severity: f.Severity, Side: f.Side, Title: f.Title, Impact: f.Impact, Location: f.Location})
	}
	return &v1.GetReportResponse{Report: out}, nil
}
