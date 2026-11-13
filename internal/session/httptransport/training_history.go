package httptransport

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

const (
	trainingHistoryPath = "/api/v1/training/submissions"
	historyCursorTime   = "2006-01-02T15:04:05.000000Z"
	historyCursorBytes  = len(historyCursorTime) + 1 + 32
)

// TrainingHistory authorizes a fresh ordinary session observation and reads a
// page in the same business transaction. It returns summaries after commit.
type TrainingHistory func(context.Context, identity.TrainingActor, *submission.HistoryPosition) (submission.HistoryPage, error)

// WithTrainingHistory enables the optional owner-only training history route.
func WithTrainingHistory(base http.Handler, history TrainingHistory) (http.Handler, error) {
	h, ok := base.(*handler)
	if !ok || h == nil || history == nil || h.trainingHistory != nil {
		return nil, redisstore.ErrInvalid
	}
	next := *h
	next.trainingHistory = history
	return &next, nil
}

// IsTrainingHistoryPath recognizes the exact collection path for delegation.
func IsTrainingHistoryPath(path string) bool { return path == trainingHistoryPath }

func (h *handler) serveTrainingHistory(w http.ResponseWriter, r *http.Request) {
	expected, pinned := expectedSubject(r)
	cursor, queryOK := listCursor(r)
	after, cursorOK := decodeHistoryCursor(cursor)
	if !pinned || !queryOK || !cursorOK || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		problem(w, 400, "INVALID_HISTORY_REQUEST", "提交历史请求格式无效")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	actor, ok := h.trainingActor(ctx, w, r, expected, historyUnavailable)
	if !ok {
		return
	}
	page, err := h.trainingHistory(ctx, actor, after)
	if ctx.Err() != nil {
		historyUnavailable(w)
		return
	}
	if err != nil {
		trainingError(w, err, historyUnavailable)
		return
	}
	next := ""
	if page.Next != nil {
		next = encodeHistoryCursor(*page.Next)
		if next == "" {
			historyUnavailable(w)
			return
		}
	}
	body, err := json.Marshal(struct {
		Submissions []submission.Receipt `json:"submissions"`
		NextCursor  string               `json:"next_cursor"`
	}{page.Items, next})
	if err != nil || ctx.Err() != nil {
		historyUnavailable(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (h *handler) trainingActor(ctx context.Context, w http.ResponseWriter, r *http.Request, expected string, unavailable func(http.ResponseWriter)) (identity.TrainingActor, bool) {
	credential, err := requestCredential(r)
	if err != nil {
		sessionError(w, redisstore.ErrNotFound)
		return identity.TrainingActor{}, false
	}
	view, err := h.store.Read(ctx, credential)
	if err != nil {
		sessionError(w, err)
		return identity.TrainingActor{}, false
	}
	actor := identity.TrainingActor{SubjectID: view.SubjectID, SessionVersion: identity.SessionVersion{
		AuthGeneration: view.AuthGeneration, RecoveryID: view.RecoveryID,
	}}
	if !actor.Valid() || ctx.Err() != nil {
		unavailable(w)
		return identity.TrainingActor{}, false
	}
	if actor.SubjectID != expected {
		problem(w, 409, "TRAINING_SUBJECT_CHANGED", "会话用户已变化")
		return identity.TrainingActor{}, false
	}
	return actor, true
}

func trainingError(w http.ResponseWriter, err error, unavailable func(http.ResponseWriter)) {
	switch {
	case errors.Is(err, identity.ErrStore), errors.Is(err, identity.ErrRecovering):
		unavailable(w)
	case errors.Is(err, identity.ErrTrainingAccountRequired):
		problem(w, 403, "TRAINING_ACCOUNT_REQUIRED", "训练需要已验证邮箱的个人账号")
	case sessionDenied(err):
		sessionError(w, err)
	default:
		unavailable(w)
	}
}

func historyUnavailable(w http.ResponseWriter) {
	problem(w, 503, "TRAINING_HISTORY_UNAVAILABLE", "提交历史暂时不可用")
}

func encodeHistoryCursor(position submission.HistoryPosition) string {
	stamp := position.CreatedAt.UTC().Format(historyCursorTime)
	if position.CreatedAt.IsZero() || position.CreatedAt.Year() < 1 || len(stamp) != len(historyCursorTime) ||
		position.CreatedAt.Nanosecond()%1000 != 0 || !identity.ValidID(position.ID) {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(stamp + "." + position.ID))
}

func decodeHistoryCursor(cursor string) (*submission.HistoryPosition, bool) {
	if cursor == "" {
		return nil, true
	}
	if len(cursor) != base64.RawURLEncoding.EncodedLen(historyCursorBytes) {
		return nil, false
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(cursor)
	if err != nil || len(raw) != historyCursorBytes {
		return nil, false
	}
	stamp, id, ok := strings.Cut(string(raw), "Z.")
	if !ok {
		return nil, false
	}
	t, err := time.Parse(historyCursorTime, stamp+"Z")
	position := submission.HistoryPosition{CreatedAt: t, ID: id}
	return &position, err == nil && encodeHistoryCursor(position) == cursor
}
