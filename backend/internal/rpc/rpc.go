// Package rpc implements the Connect services of the API contract (proto/aicompare/v1).
package rpc

import (
	"net/http"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	"ai-compare/backend/internal/catalog"
	"ai-compare/backend/internal/comparison"
	"ai-compare/backend/internal/gen/aicompare/v1/aicomparev1connect"
	"ai-compare/backend/internal/presets"
	"ai-compare/backend/internal/report"
	"ai-compare/backend/internal/settings"
	"ai-compare/backend/internal/workspace"
)

// Deps are what the services need. Workspace, Comparisons and Reports are nil when Docker is not
// reachable; their services are then not registered.
type Deps struct {
	Catalog     *catalog.Service
	Settings    *settings.Service
	Workspace   *workspace.Service
	Comparisons *comparison.Service
	Reports     *report.Service
	Presets     *presets.Store
	// HostHome is where the folder browser starts.
	HostHome string
	Env      Env
}

// Handler serves every Connect service. The caller mounts it under a prefix it strips,
// e.g. /api/rpc/aicompare.v1.CatalogService/GetCatalog.
func Handler(d Deps) http.Handler {
	server := connect.NewServer()
	aicomparev1connect.RegisterCatalogServiceHandler(server, &catalogService{models: d.Catalog})
	aicomparev1connect.RegisterSettingsServiceHandler(server, &settingsService{settings: d.Settings, comparisons: d.Comparisons, ws: d.Workspace, env: d.Env})
	if d.Workspace != nil {
		aicomparev1connect.RegisterProjectServiceHandler(server, &projectService{ws: d.Workspace, home: d.HostHome})
	}
	if d.Comparisons != nil {
		aicomparev1connect.RegisterComparisonServiceHandler(server, &comparisonService{svc: d.Comparisons})
		aicomparev1connect.RegisterEventServiceHandler(server, &eventService{svc: d.Comparisons})
	}
	if d.Presets != nil {
		aicomparev1connect.RegisterPresetServiceHandler(server, &presetService{store: d.Presets, comparisons: d.Comparisons, ws: d.Workspace})
	}
	if d.Reports != nil {
		aicomparev1connect.RegisterReportServiceHandler(server, &reportService{svc: d.Reports})
	}
	mux := http.NewServeMux()
	connecthttp.Mount(mux, server)
	return mux
}
