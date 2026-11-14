package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	problemmodel "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

const trainingReceiptPath = "/api/v1/training/receipt"

// TrainingSubmit accepts a submission to an explicitly published version.
type TrainingSubmit func(context.Context, identity.TrainingActor, string, string, string, string) (submission.Receipt, error)

// TrainingReceipt recovers a previously accepted submission by its request key.
type TrainingReceipt func(context.Context, identity.TrainingActor, string) (submission.Receipt, error)

// IsTrainingReceiptPath identifies the owner-only recovery endpoint.
func IsTrainingReceiptPath(path string) bool { return path == trainingReceiptPath }

// WithTrainingIntake enables submit and recovery together.
func WithTrainingIntake(base http.Handler, submit TrainingSubmit, receipt TrainingReceipt) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || submit == nil || receipt == nil || h.trainingSubmit != nil {
		return nil, redisstore.ErrInvalid
	}
	next := *h
	next.trainingSubmit, next.trainingReceipt = submit, receipt
	return &next, nil
}

func (h *handler) serveTrainingIntake(w http.ResponseWriter, r *http.Request) {
	expected, pinned := expectedSubject(r)
	keys := r.Header.Values("Idempotency-Key")
	if !pinned || len(keys) != 1 || !submission.ValidKey(keys[0]) || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		invalidTrainingIntake(w)
		return
	}
	key := keys[0]
	post := r.Method == http.MethodPost
	var id, version, source string
	if post {
		if len(r.Header.Values("Content-Type")) != 1 {
			problem(w, 415, "UNSUPPORTED_MEDIA_TYPE", "请使用 JSON 提交")
			return
		}
		if !readReviewFields(w, r, submission.MaxSourceBytes*6+512, map[string]any{
			"problem_id": &id, "publication_version": &version, "source": &source,
		}, invalidTrainingIntake) {
			return
		}
		if !identity.ValidID(id) || !problemmodel.ValidImportRevision(version) ||
			submission.Validate(expected, submission.PersonalTrainingScope, key, submission.Request{
				MaterialVersion: version, Language: "gnu++17", Source: source,
			}) != nil {
			invalidTrainingIntake(w)
			return
		}
	} else if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		invalidTrainingIntake(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	actor, ok := h.trainingActor(ctx, w, r, expected, trainingIntakeUnavailable)
	if !ok {
		return
	}
	var receipt submission.Receipt
	var err error
	if post {
		receipt, err = h.trainingSubmit(ctx, actor, key, id, version, source)
	} else {
		receipt, err = h.trainingReceipt(ctx, actor, key)
	}
	if ctx.Err() != nil {
		trainingIntakeUnavailable(w)
		return
	}
	switch {
	case errors.Is(err, submission.ErrNotFound):
		if post {
			problem(w, 404, "TRAINING_PUBLICATION_UNAVAILABLE", "此题目版本暂不可提交")
		} else {
			problem(w, 404, "TRAINING_RECEIPT_NOT_FOUND", "暂未查到收件回执")
		}
	case errors.Is(err, submission.ErrConflict):
		problem(w, 409, "TRAINING_KEY_CONFLICT", "提交请求键已用于不同内容")
	case errors.Is(err, submission.ErrBacklog):
		problem(w, 429, "TRAINING_BACKLOG_LIMIT", "提交队列已满，请稍后重试")
	case err != nil:
		trainingError(w, err, trainingIntakeUnavailable)
	default:
		body, err := json.Marshal(receipt)
		if err != nil {
			trainingIntakeUnavailable(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}

func invalidTrainingIntake(w http.ResponseWriter) {
	problem(w, 400, "INVALID_TRAINING_INTAKE", "训练提交请求格式无效")
}

func trainingIntakeUnavailable(w http.ResponseWriter) {
	problem(w, 503, "TRAINING_INTAKE_UNAVAILABLE", "暂时无法确认训练提交")
}
