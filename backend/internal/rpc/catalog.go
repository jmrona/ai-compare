// Package rpc implements the Connect services of the API contract (proto/aicompare/v1).
package rpc

import (
	"context"
	"net/http"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"google.golang.org/protobuf/types/known/timestamppb"

	"ai-compare/backend/internal/catalog"
	v1 "ai-compare/backend/internal/gen/aicompare/v1"
	"ai-compare/backend/internal/gen/aicompare/v1/aicomparev1connect"
	"ai-compare/backend/internal/workspace"
)

// Handler serves every Connect service. The caller mounts it under a prefix it strips,
// e.g. /api/rpc/aicompare.v1.CatalogService/GetCatalog.
func Handler(models *catalog.Service, ws *workspace.Service, hostHome string) http.Handler {
	server := connect.NewServer()
	aicomparev1connect.RegisterCatalogServiceHandler(server, &catalogService{models: models})
	if ws != nil {
		aicomparev1connect.RegisterProjectServiceHandler(server, &projectService{ws: ws, home: hostHome})
	}
	mux := http.NewServeMux()
	connecthttp.Mount(mux, server)
	return mux
}

type catalogService struct {
	models *catalog.Service
}

func (s *catalogService) GetCatalog(ctx context.Context, _ *v1.GetCatalogRequest) (*v1.GetCatalogResponse, error) {
	c, err := s.models.Get(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err.Error())
	}
	return &v1.GetCatalogResponse{Catalog: catalogToProto(c)}, nil
}

func (s *catalogService) RefreshCatalog(ctx context.Context, _ *v1.RefreshCatalogRequest) (*v1.RefreshCatalogResponse, error) {
	c, err := s.models.Refresh(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err.Error())
	}
	return &v1.RefreshCatalogResponse{Catalog: catalogToProto(c)}, nil
}

func catalogToProto(c catalog.Catalog) *v1.Catalog {
	out := &v1.Catalog{
		Source: c.Source, FetchedAt: timestamppb.New(c.FetchedAt), FromCache: c.FromCache, Warning: c.Warning,
		Models: make([]*v1.Model, 0, len(c.Models)),
	}
	for _, m := range c.Models {
		pm := &v1.Model{
			Id: m.ID, Name: m.Name, Provider: m.Provider, Family: m.Family, ReleaseDate: m.ReleaseDate,
			Deprecated: m.Deprecated, ToolCall: m.ToolCall, TextOutput: m.TextOutput, Efforts: m.Efforts,
			ContextK: int32(m.ContextK), Price: priceToProto(m.Price),
		}
		if m.LongContext != nil {
			pm.LongContext = &v1.LongContext{AboveTokens: int32(m.LongContext.AboveTokens), Price: priceToProto(&m.LongContext.Price)}
		}
		out.Models = append(out.Models, pm)
	}
	return out
}

func priceToProto(p *catalog.Price) *v1.Price {
	if p == nil {
		return nil
	}
	return &v1.Price{Input: p.Input, CacheRead: p.CacheRead, CacheWrite: p.CacheWrite, Output: p.Output}
}
