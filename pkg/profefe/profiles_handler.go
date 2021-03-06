package profefe

import (
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/profefe/profefe/pkg/log"
	"github.com/profefe/profefe/pkg/profile"
	"github.com/profefe/profefe/pkg/storage"
	"golang.org/x/xerrors"
)

type ProfilesHandler struct {
	logger    *log.Logger
	collector *Collector
	querier   *Querier
}

func NewProfilesHandler(logger *log.Logger, collector *Collector, querier *Querier) *ProfilesHandler {
	return &ProfilesHandler{
		logger:    logger,
		collector: collector,
		querier:   querier,
	}
}

func (h *ProfilesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var (
		handler func(http.ResponseWriter, *http.Request) error
		urlPath = path.Clean(r.URL.Path)
	)

	if urlPath == apiProfilesMergePath {
		handler = h.HandleMergeProfile
	} else if urlPath == apiProfilesPath {
		switch r.Method {
		case http.MethodPost:
			handler = h.HandleCreateProfile
		case http.MethodGet:
			handler = h.HandleFindProfiles
		}
	} else if len(urlPath) > len(apiProfilesPath) {
		handler = h.HandleGetProfile
	}

	var err error
	if handler != nil {
		err = handler(w, r)
	} else {
		err = ErrNotFound
	}
	HandleErrorHTTP(h.logger, err, w, r)
}

func (h *ProfilesHandler) HandleCreateProfile(w http.ResponseWriter, r *http.Request) error {
	params := &storage.WriteProfileParams{}
	if err := parseWriteProfileParams(params, r); err != nil {
		return err
	}

	profModel, err := h.collector.WriteProfile(r.Context(), params, r.Body)
	if err != nil {
		return StatusError(http.StatusInternalServerError, "failed to collect profile", err)
	}

	ReplyJSON(w, profModel)

	return nil
}

func (h *ProfilesHandler) HandleGetProfile(w http.ResponseWriter, r *http.Request) error {
	pid := r.URL.Path[len(apiProfilesPath):] // id part of the path
	pid = strings.Trim(pid, "/")
	if pid == "" {
		return StatusError(http.StatusBadRequest, "no profile id", nil)
	}

	w.Header().Set("Content-Type", "application/octet-stream")

	err := h.querier.GetProfileTo(r.Context(), w, profile.ID(pid))
	if err == storage.ErrNotFound {
		return ErrNotFound
	} else if err == storage.ErrEmpty {
		return ErrEmpty
	} else if err != nil {
		err = xerrors.Errorf("could not get profile by id %q: %w", pid, err)
		return StatusError(http.StatusInternalServerError, fmt.Sprintf("failed to get profile by id %q", pid), err)
	}
	return nil
}

func (h *ProfilesHandler) HandleFindProfiles(w http.ResponseWriter, r *http.Request) error {
	params := &storage.FindProfilesParams{}
	if err := parseFindProfileParams(params, r); err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")

	profModels, err := h.querier.FindProfiles(r.Context(), params)
	if err == storage.ErrNotFound {
		return ErrNotFound
	} else if err == storage.ErrEmpty {
		return ErrEmpty
	} else if err != nil {
		return err
	}

	ReplyJSON(w, profModels)

	return nil
}

func (h *ProfilesHandler) HandleMergeProfile(w http.ResponseWriter, r *http.Request) error {
	params := &storage.FindProfilesParams{}
	if err := parseFindProfileParams(params, r); err != nil {
		return err
	}

	if params.Type == profile.TypeTrace {
		return StatusError(http.StatusMethodNotAllowed, "tracing profiles are not mergeable", nil)
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, params.Type))

	err := h.querier.FindMergeProfileTo(r.Context(), w, params)
	if err == storage.ErrNotFound {
		return ErrNotFound
	} else if err == storage.ErrEmpty {
		return ErrEmpty
	}
	return err
}
