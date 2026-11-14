package execution

import (
	"errors"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
)

func TestCheckerReceiptRejectsFaultsEvenWithPassExit(t *testing.T) {
	budget := experiment.Limits{CPU: 100, Memory: 100}
	receipt := func(exit int) goJudgeResult {
		cpu, memory := int64(100), int64(100)
		out, diagnostic := "", "arbitrary text"
		return goJudgeResult{
			Status: "Nonzero Exit Status", Exit: &exit, CPU: &cpu, Memory: &memory,
			Files: map[string]*string{"stdout": &out, "stderr": &diagnostic},
		}
	}
	if got, err := verifyCheckerResult(receipt(50), judging.CheckerTestlib0945, budget, 16); got != judging.CheckerAccept || err != nil {
		t.Fatalf("normal Testlib result: %v, %v", got, err)
	}
	if got, err := verifyCheckerResult(receipt(52), judging.CheckerTestlib0945, budget, 16); got != judging.CheckerWrongAnswer || err != nil {
		t.Fatalf("normal contestant format error: %v, %v", got, err)
	}
	for _, tc := range []struct {
		name   string
		change func(*goJudgeResult)
	}{
		{"signal", func(r *goJudgeResult) { r.Status = "Signalled" }},
		{"timeout", func(r *goJudgeResult) { r.Status = "Time Limit Exceeded" }},
		{"output limit", func(r *goJudgeResult) { r.Status = "Output Limit Exceeded" }},
		{"contradictory accepted status", func(r *goJudgeResult) { r.Status = "Accepted" }},
		{"ordinary process exit", func(r *goJudgeResult) { *r.Exit = 0 }},
		{"inconsistent CPU", func(r *goJudgeResult) { *r.CPU = 101 }},
		{"inconsistent memory", func(r *goJudgeResult) { *r.Memory = 101 }},
		{"missing resource evidence", func(r *goJudgeResult) { r.Memory = nil }},
		{"collector fault", func(r *goJudgeResult) {
			r.FileErrors = append(r.FileErrors, struct {
				Name string `json:"name"`
				Type string `json:"type"`
			}{"stderr", "CollectSizeExceeded"})
		}},
		{"oversized diagnostic", func(r *goJudgeResult) { *r.Files["stderr"] = "oversized diagnostic" }},
		{"missing diagnostic receipt", func(r *goJudgeResult) { delete(r.Files, "stderr") }},
		{"unexpected artifact", func(r *goJudgeResult) { r.IDs = map[string]string{"answer": "extra"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := receipt(50)
			tc.change(&r)
			if got, err := verifyCheckerResult(r, judging.CheckerTestlib0945, budget, 16); got != judging.CheckerSystemError ||
				!errors.Is(err, judging.ErrCheckerResult) {
				t.Fatalf("invalid checker receipt: %v, %v", got, err)
			}
		})
	}
}
