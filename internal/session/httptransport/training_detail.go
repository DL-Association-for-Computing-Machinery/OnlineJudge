package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

// TrainingDetail rechecks training eligibility while reading an owner's submission.
type TrainingDetail func(context.Context, identity.TrainingActor, string) (submission.Detail, error)

// TrainingStatus reads the same current receipt without loading the source.
type TrainingStatus func(context.Context, identity.TrainingActor, string) (submission.Receipt, error)

// WithTrainingDetail enables the optional read-only submission detail route.
func WithTrainingDetail(base http.Handler, read TrainingDetail, status TrainingStatus) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || read == nil || status == nil || h.trainingDetail != nil {
		return nil, redisstore.ErrInvalid
	}
	next := *h
	next.trainingDetail = read
	next.trainingStatus = status
	return &next, nil
}

// IsTrainingDetailPath recognizes the submission item path for delegation.
func IsTrainingDetailPath(path string) bool { return trainingDetailID(path) != "" }

func trainingDetailID(path string) string {
	id, ok := strings.CutPrefix(path, trainingHistoryPath+"/")
	if !ok || !identity.ValidID(id) {
		return ""
	}
	return id
}

func (h *handler) serveTrainingDetail(w http.ResponseWriter, r *http.Request, id string) {
	expected, pinned := expectedSubject(r)
	statusOnly := r.URL.RawQuery == "view=status"
	if !pinned || r.URL.RawPath != "" || (r.URL.RawQuery != "" && !statusOnly) || r.URL.ForceQuery ||
		r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "INVALID_TRAINING_DETAIL_REQUEST", "提交详情请求格式无效")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	actor, ok := h.trainingActor(ctx, w, r, expected, detailUnavailable)
	if !ok {
		return
	}
	var result any
	var err error
	if statusOnly {
		result, err = h.trainingStatus(ctx, actor, id)
	} else {
		result, err = h.trainingDetail(ctx, actor, id)
	}
	if ctx.Err() != nil {
		detailUnavailable(w)
		return
	}
	if errors.Is(err, submission.ErrNotFound) {
		problem(w, 404, "NOT_FOUND", "提交不存在")
		return
	}
	if err != nil {
		trainingError(w, err, detailUnavailable)
		return
	}
	body, err := json.Marshal(result)
	if err != nil || ctx.Err() != nil {
		detailUnavailable(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func detailUnavailable(w http.ResponseWriter) {
	problem(w, 503, "TRAINING_DETAIL_UNAVAILABLE", "提交详情暂时不可用")
}
