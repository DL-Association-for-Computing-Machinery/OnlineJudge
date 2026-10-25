package httpapi_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/httpapi"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission"
)

type sourceStore struct {
	calls  int
	source string
}

func (s *sourceStore) Accept(_ context.Context, subject, scope, key string, req submission.Request) (submission.Receipt, bool, error) {
	s.calls++
	if err := submission.Validate(subject, scope, key, req); err != nil {
		return submission.Receipt{}, false, err
	}
	s.source = req.Source
	return submission.Receipt{ID: "synthetic", State: "queued"}, true, nil
}

func (*sourceStore) Receipt(context.Context, string, string, string) (submission.Receipt, error) {
	return submission.Receipt{}, submission.ErrNotFound
}

func (*sourceStore) Get(context.Context, string, string, string) (submission.Receipt, error) {
	return submission.Receipt{}, submission.ErrNotFound
}

func TestExperimentUnicodePreservation(t *testing.T) {
	for _, tc := range []struct {
		name, sourceJSON, want string
		valid                  bool
	}{
		{"unpaired high", `"//\ud800"`, "", false},
		{"unpaired low", `"//\udfff"`, "", false},
		{"two high surrogates", `"\ud800\ud801"`, "", false},
		{"high then ordinary escape", `"\ud800\u0061"`, "", false},
		{"separated pair", `"\ud800x\udc00"`, "", false},
		{"truncated escape", `"\ud80"`, "", false},
		{"invalid raw UTF8", "\"\xff\"", "", false},
		{"paired surrogate", `"//\ud83d\ude00"`, "//😀", true},
		{"uppercase pair", `"\uD800\uDC00"`, "𐀀", true},
		{"literal escape text", `"//\\ud800"`, `//\ud800`, true},
		{"escaped slash before invalid surrogate", `"\\\ud800"`, "", false},
		{"replacement character", `"//\ufffd"`, "//�", true},
		{"raw Unicode", `"// 中文😀"`, "// 中文😀", true},
		{"BMP and control escapes", `"\u4e2d\u6587\n\t\"\\"`, "中文\n\t\"\\", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &sourceStore{}
			const token = "synthetic-test-only-token-not-a-real-credential"
			handler := httpapi.NewHandler(httpapi.Experiment{Store: store, Token: token})
			body := `{"material_version":"m0-sum-v1","language":"gnu++17","source":` + tc.sourceJSON + `}`
			request := httptest.NewRequest(http.MethodPost, "/api/v1/experiments/submissions", strings.NewReader(body))
			request.Header.Set("Authorization", "Bearer "+token)
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", "unicode-fixture-0001")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if !tc.valid {
				if response.Code != http.StatusBadRequest || store.calls != 0 {
					t.Fatalf("invalid source reached persistence: status=%d calls=%d", response.Code, store.calls)
				}
			} else if response.Code != http.StatusAccepted || store.calls != 1 || store.source != tc.want {
				t.Fatalf("source changed: status=%d calls=%d source=%q want=%q", response.Code, store.calls, store.source, tc.want)
			}
		})
	}
}
