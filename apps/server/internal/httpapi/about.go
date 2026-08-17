package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/buildinfo"
)

// AboutResponse is the payload of GET /api/about.
//
// Every field here is either a fixed product-identity constant
// (internal/buildinfo) or build metadata Go can determine reliably on its
// own. Display prose ("Development build", licence status wording) is
// deliberately NOT sent as English text here - that is UI copy and belongs
// in the frontend's own localized strings, keyed off the status
// fields below.
//
// This response must never contain a Git author email, OS username,
// hostname, filesystem path, credential, token, or database path.
type AboutResponse struct {
	ProductName    string `json:"productName"`
	Version        string `json:"version"`
	IsReleaseBuild bool   `json:"isReleaseBuild"`
	// Commit and CommitDirty are omitted entirely when no VCS revision is
	// available, rather than sent as empty/false placeholders that could be
	// mistaken for a real (empty) commit.
	Commit                   string `json:"commit,omitempty"`
	CommitDirty              bool   `json:"commitDirty,omitempty"`
	CreatorName              string `json:"creatorName"`
	RepositoryURL            string `json:"repositoryUrl"`
	CreatorURL               string `json:"creatorUrl"`
	SupportURL               string `json:"supportUrl"`
	ApplicationLicenceStatus string `json:"applicationLicenceStatus"`
}

// aboutHandler reports fixed product-identity and build metadata for the
// About & Legal UI. It takes no dependencies beyond the logger: every field
// is a compile-time constant or something Go's own build-info stamping
// already knows, so unlike most routes in this package it needs no service
// interface and is always registered.
func aboutHandler(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		commit, dirty, ok := buildinfo.CommitInfo()

		resp := AboutResponse{
			ProductName:              buildinfo.ProductName,
			Version:                  buildinfo.Version,
			IsReleaseBuild:           buildinfo.IsReleaseBuild,
			CreatorName:              buildinfo.CreatorName,
			RepositoryURL:            buildinfo.RepositoryURL,
			CreatorURL:               buildinfo.CreatorURL,
			SupportURL:               buildinfo.SupportURL,
			ApplicationLicenceStatus: buildinfo.ApplicationLicenceStatus,
		}
		if ok {
			resp.Commit = commit
			resp.CommitDirty = dirty
		}

		writeJSON(w, logger, http.StatusOK, resp)
	}
}
