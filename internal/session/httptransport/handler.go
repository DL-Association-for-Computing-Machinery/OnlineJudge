// Package httptransport adapts ordinary sessions and two-step login to HTTPS.
// It grants no email, role, training, or resource permissions.
package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
)

// CookieName is reserved for host-only, secure ordinary session credentials.
const CookieName = "__Host-oj_session"

// Store must provide fresh authority checks, explicit renewal and confirmed
// revocation, as implemented by redisstore.Store. Calls never imply a retry.
type Store interface {
	Read(context.Context, redisstore.Credential) (redisstore.View, error)
	Touch(context.Context, redisstore.Credential) (redisstore.View, error)
	Revoke(context.Context, redisstore.Credential) error
}

// Config freezes the trusted public HTTPS origin and a cooperative call budget.
// TLS is always required; forwarding trust needs the explicit proxy constructor.
type Config struct {
	Origin  string
	Timeout time.Duration
}

type handler struct {
	store                  Store
	config                 Config
	host                   string
	login                  *redisstore.Login
	proxy                  *proxyTrust
	passwordChange         *redisstore.PasswordChange
	emailConfirmation      EmailConfirmation
	emailRequest           EmailRequest
	drafts                 Drafts
	draftTimeout           time.Duration
	references             ReferenceRequests
	preparation            ReferencePreparation
	reviewIntake           ReviewIntake
	reviewEvidence         ReviewEvidenceReader
	reviewSolution         AuthorReviewSolution
	reviewMaterials        ReviewMaterialsReader
	reviewReturns          ReviewReturns
	statementConfirmations StatementConfirmations
}

// New builds three explicit endpoints without registering any application route:
// GET /api/v1/session, POST /api/v1/session/activity and POST /api/v1/session/logout.
// Forwarding headers have no authority in this direct constructor.
func New(store Store, config Config) (http.Handler, error) {
	if store == nil || config.Validate() != nil {
		return nil, errors.New("invalid session HTTP configuration")
	}
	u, _ := url.Parse(config.Origin)
	return &handler{store: store, config: config, host: u.Host}, nil
}

// Validate checks HTTP configuration without a store, key material or I/O.
func (config Config) Validate() error {
	u, err := url.Parse(config.Origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Hostname() == "" ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		u.Opaque != "" || config.Origin != "https://"+u.Host || config.Timeout <= 0 || config.Timeout > 5*time.Second {
		return errors.New("invalid session HTTP configuration")
	}
	return nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if IsEmailConfirmationPath(r.URL.Path) || IsEmailRequestPath(r.URL.Path) {
		w.Header().Set("Referrer-Policy", "no-referrer")
	}
	source, trusted := h.requestSource(r)
	if !trusted {
		problem(w, http.StatusForbidden, "ORIGIN_REJECTED", "请求来源不允许")
		return
	}
	method := ""
	isDraft := IsPrivateDraftPath(r.URL.Path)
	refMethod, refKey := referenceRequestRoute(r.URL.Path)
	reviewMethod, reviewKey, authorAction := reviewIntakeRoute(r.URL.Path)
	reviewID, reviewRead := reviewReadRoute(r.URL.Path)
	returnID, returnKey, returnMethod := reviewReturnRoute(r.URL.Path)
	statementID, statementKey, statementMethod := reviewActionRoute(r.URL.Path, statementConfirmationAction)
	switch r.URL.Path {
	case "/api/v1/session":
		method = http.MethodGet
	case "/api/v1/session/activity", "/api/v1/session/logout":
		method = http.MethodPost
	case "/api/v1/session/login/begin", "/api/v1/session/login/complete", "/api/v1/session/login/abandon":
		if h.login == nil {
			problem(w, 404, "NOT_FOUND", "接口不存在")
			return
		}
		method = http.MethodPost
	case "/api/v1/account/password":
		if h.passwordChange == nil {
			problem(w, 404, "NOT_FOUND", "接口不存在")
			return
		}
		method = http.MethodPost
	case emailActivationPath, emailResetPath:
		if h.emailConfirmation == nil {
			problem(w, 404, "NOT_FOUND", "接口不存在")
			return
		}
		method = http.MethodPost
	case emailRegisterPath, emailRecoveryRequestPath:
		if h.emailRequest == nil {
			problem(w, 404, "NOT_FOUND", "接口不存在")
			return
		}
		method = http.MethodPost
	default:
		if statementMethod != "" {
			if h.statementConfirmations == nil {
				problem(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
				return
			}
			method = statementMethod
			break
		}
		if returnMethod != "" {
			if h.reviewReturns == nil {
				problem(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
				return
			}
			method = returnMethod
			break
		}
		if reviewID != "" {
			if h.reviewEvidence == nil || (reviewRead == "materials" && h.reviewMaterials == nil) {
				problem(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
				return
			}
			method = http.MethodGet
			break
		}
		if reviewMethod != "" {
			if h.reviewIntake == nil || ((authorAction == "solution" || authorAction == "solution-confirmation") && h.reviewSolution == nil) {
				problem(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
				return
			}
			method = reviewMethod
			if authorAction == "solution-confirmation" && r.Method == http.MethodPost {
				method = http.MethodPost
			}
			break
		}
		if refMethod != "" && h.references != nil {
			method = refMethod
			break
		}
		if !isDraft || h.drafts == nil {
			problem(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
			return
		}
		if _, available := h.drafts.(DraftPreview); draftPreviewID(r.URL.Path) != "" && !available {
			problem(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
			return
		}
		if _, available := h.drafts.(DraftImportProtocol); draftImportPath(r.URL.Path) && !available {
			problem(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
			return
		}
		if _, available := h.drafts.(DraftManagement); managementPath(r.URL.Path) && !available {
			problem(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
			return
		}
		if _, available := h.drafts.(OwnDrafts); r.URL.Path == ownDraftCollection && !available {
			problem(w, http.StatusNotFound, "NOT_FOUND", "接口不存在")
			return
		}
		method = http.MethodGet
		if r.URL.Path == draftCollection || r.URL.Path == draftImports || r.URL.Path == managementImports {
			method = http.MethodPost
		}
	}
	if r.Method != method {
		allow := method
		if authorAction == "solution-confirmation" {
			allow = "GET, POST"
		}
		w.Header().Set("Allow", allow)
		problem(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "请求方法不支持")
		return
	}
	if method == http.MethodPost && !h.sameOrigin(r) {
		problem(w, http.StatusForbidden, "ORIGIN_REJECTED", "请求来源不允许")
		return
	}
	if refMethod != "" {
		h.serveReferenceRequest(w, r, refKey)
		return
	}
	if reviewMethod != "" {
		if authorAction == "solution" || authorAction == "solution-confirmation" {
			h.serveAuthorReviewSolution(w, r, reviewKey, authorAction)
		} else {
			h.serveReviewIntake(w, r, reviewKey, authorAction == "return")
		}
		return
	}
	if returnMethod != "" {
		h.serveReviewReturn(w, r, returnID, returnKey)
		return
	}
	if statementMethod != "" {
		h.serveStatementConfirmation(w, r, statementID, statementKey)
		return
	}
	if reviewID != "" {
		h.serveReviewRead(w, r, reviewID, reviewRead)
		return
	}
	if isDraft {
		h.serveDraft(w, r)
		return
	}
	if r.URL.Path == "/api/v1/account/password" {
		h.servePasswordChange(w, r, source)
		return
	}
	if IsEmailConfirmationPath(r.URL.Path) {
		h.serveEmailConfirmation(w, r, source)
		return
	}
	if IsEmailRequestPath(r.URL.Path) {
		h.serveEmailRequest(w, r, source)
		return
	}
	if h.login != nil && (r.URL.Path == "/api/v1/session/login/begin" || r.URL.Path == "/api/v1/session/login/complete" || r.URL.Path == "/api/v1/session/login/abandon") {
		h.serveLogin(w, r, source)
		return
	}
	credential, err := requestCredential(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "SESSION_REQUIRED", "会话无效或已过期")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.config.Timeout)
	defer cancel()
	var view redisstore.View
	switch r.URL.Path {
	case "/api/v1/session":
		view, err = h.store.Read(ctx, credential)
	case "/api/v1/session/activity":
		view, err = h.store.Touch(ctx, credential)
	case "/api/v1/session/logout":
		err = h.store.Revoke(ctx, credential)
	}
	// Cancellation does not prove rollback, including after a nil Store result.
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		sessionError(w, err)
		return
	}
	if r.URL.Path == "/api/v1/session/logout" {
		http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", Secure: true,
			HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0).UTC()})
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Internal auth/recovery generations never cross the transport boundary.
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		SubjectID        string    `json:"subject_id"`
		CreatedAt        time.Time `json:"created_at"`
		IdleExpiresAt    time.Time `json:"idle_expires_at"`
		AbsoluteDeadline time.Time `json:"absolute_deadline"`
	}{view.SubjectID, view.CreatedAt, view.IdleExpiresAt, view.AbsoluteDeadline})
}

func sessionError(w http.ResponseWriter, err error) {
	status, code, title := http.StatusServiceUnavailable, "SESSION_UNAVAILABLE", "会话服务暂时不可用"
	if sessionDenied(err) {
		status, code, title = http.StatusUnauthorized, "SESSION_REQUIRED", "会话无效或已过期"
	}
	problem(w, status, code, title)
}

func sessionDenied(err error) bool {
	// Unknown effects always win. Explicit protocol refusal may deny identity
	// even inside a guard, but never confirms logout or rollback of an effect.
	failed := errors.Is(err, identity.ErrStore) || errors.Is(err, redisstore.ErrStore) ||
		errors.Is(err, identity.ErrRecovering) || errors.Is(err, redisstore.ErrUnknown) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	guarded := errors.Is(err, identity.ErrGuardedOperation)
	refused := errors.Is(err, redisstore.ErrStale) || errors.Is(err, redisstore.ErrNotFound)
	return !failed && (refused || (!guarded && (errors.Is(err, identity.ErrConflict) || errors.Is(err, identity.ErrNotFound))))
}

func problem(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Status int    `json:"status"`
		Code   string `json:"code"`
	}{"about:blank", title, status, code})
}
