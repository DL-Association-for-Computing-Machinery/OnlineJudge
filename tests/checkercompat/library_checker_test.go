//go:build checkercompat

// Package checkercompat_test runs pinned third-party code only in the dedicated
// offline, unprivileged Docker profile documented in docs/checker-admission.md.
package checkercompat_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
)

type identity struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type sourceLock struct {
	Commit   string              `json:"commit"`
	Files    map[string]identity `json:"files"`
	Problems []struct {
		Path string `json:"path"`
	} `json:"problems"`
}

type testCase struct{ name, input, answer, inputHash, answerHash string }

func read(t *testing.T, path string, limit int64) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		t.Fatalf("invalid regular file: %s", path)
	}
	raw, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(raw)) > limit {
		t.Fatalf("bounded read failed: %s", path)
	}
	return raw
}

func verified(t *testing.T, path string, want identity) []byte {
	t.Helper()
	raw := read(t, path, judging.MaxTextBytes)
	h := sha256.Sum256(raw)
	if int64(len(raw)) != want.Size || hex.EncodeToString(h[:]) != want.SHA256 {
		t.Fatalf("fixed material mismatch: %s", path)
	}
	return raw
}

func write(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func child(t *testing.T, timeout time.Duration, command string, args ...string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/work", "TMPDIR=/work", "LANG=C.UTF-8"}
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("bounded child timed out: %s", command)
	}
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return exit.ExitCode()
}

func TestPinnedLibraryChecker(t *testing.T) {
	if os.Getenv("OJ_CHECKER_COMPAT") != "isolated-docker" || os.Getenv("TMPDIR") != "/work" {
		t.Fatal("requires the explicit isolated Docker command; never run third-party checkers on the host")
	}
	if _, err := os.Stat("/.dockerenv"); err != nil {
		t.Fatal("container marker missing")
	}
	lockBytes := read(t, "../../config/problem-sources/library-checker.json", 1<<20)
	var lock sourceLock
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.Commit != "73876f1cbcf082d2b22722924b924d3d65a2f00e" || len(lock.Problems) != 10 {
		t.Fatal("review the changed source lock before running")
	}
	work := t.TempDir()
	header := verified(t, "/corpus/source/common/testlib.h", lock.Files["common/testlib.h"])
	write(t, filepath.Join(work, "testlib.h"), header)
	limits := judging.TextLimits{Bytes: 16 << 20, TokenBytes: 1 << 16}
	var cases []testCase
	var checker []byte
	total, maxAnswer, overM0 := int64(0), 0, 0
	// All references must fit the declared domain before any differential result
	// is reported. Source preparation's validation.json is deliberately unused.
	for _, p := range lock.Problems {
		code := verified(t, "/corpus/source/"+p.Path+"/checker.cpp", lock.Files[p.Path+"/checker.cpp"])
		if checker != nil && !bytes.Equal(checker, code) {
			t.Fatal("checker families differ; separate admission required")
		}
		checker = code
		hashesRaw := verified(t, "/corpus/source/"+p.Path+"/hash.json", lock.Files[p.Path+"/hash.json"])
		var hashes map[string]string
		if err := json.Unmarshal(hashesRaw, &hashes); err != nil {
			t.Fatal(err)
		}
		pid := filepath.Base(p.Path)
		var names []string
		for name := range hashes {
			if strings.HasSuffix(name, ".out") {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			stem := strings.TrimSuffix(name, ".out")
			c := testCase{pid + "/" + stem, "/corpus/problems/" + pid + "/in/" + stem + ".in", "/corpus/problems/" + pid + "/out/" + name, hashes[stem+".in"], hashes[name]}
			answer := read(t, c.answer, judging.MaxTextBytes)
			checkHash(t, answer, c.answerHash)
			checkHash(t, read(t, c.input, judging.MaxTextBytes), c.inputHash)
			if ok, err := judging.CompareLibraryCheckerText(answer, answer, limits); err != nil || !ok {
				t.Fatalf("reference admission failed %s: %v", c.name, err)
			}
			total += int64(len(answer))
			maxAnswer = max(maxAnswer, len(answer))
			if len(answer) > judging.TextBudget {
				overM0++
			}
			cases = append(cases, c)
		}
	}
	if len(cases) != 254 {
		t.Fatalf("unexpected case coverage: %d", len(cases))
	}
	write(t, filepath.Join(work, "checker.cpp"), checker)
	binary := filepath.Join(work, "checker")
	if code := child(t, 120*time.Second, "/usr/bin/g++", "-O2", "-std=c++17", "-I", work, filepath.Join(work, "checker.cpp"), "-o", binary); code != 0 {
		t.Fatalf("checker compilation failed: %d", code)
	}
	input, reference, output := filepath.Join(work, "input"), filepath.Join(work, "reference"), filepath.Join(work, "output")
	write(t, input, nil) // wcmp never reads input; actual inputs were hash-verified above.
	comparisons := 0
	compare := func(label string, ref, out []byte, want bool) {
		t.Helper()
		got, err := judging.CompareLibraryCheckerText(ref, out, limits)
		if err != nil || got != want {
			t.Fatalf("%s: site=%v err=%v expected=%v", label, got, err, want)
		}
		write(t, reference, ref)
		write(t, output, out)
		checkFrozenComparison(t, reference, output, int64(len(ref)), int64(len(out)), limits, want)
		code := child(t, 5*time.Second, binary, input, output, reference)
		if code != 0 && code != 1 {
			t.Fatalf("%s: checker failure %d, not an AC/WA result", label, code)
		}
		if (code == 0) != want {
			t.Fatalf("%s: upstream=%d expected=%v", label, code, want)
		}
		comparisons++
	}
	for _, c := range cases {
		answer := read(t, c.answer, judging.MaxTextBytes)
		checkHash(t, answer, c.answerHash)
		compare(c.name+"/self", answer, answer, true)
		compare(c.name+"/whitespace", answer, append(append([]byte(" \t\r\n"), answer...), '\n'), true)
		compare(c.name+"/extra", answer, append(append([]byte(nil), answer...), []byte("\n__EXTRA__")...), false)
		compare(c.name+"/missing", answer, nil, len(bytes.TrimSpace(answer)) == 0)
	}
	for _, c := range []struct {
		name, ref, out string
		want           bool
	}{
		{"VT", "1 2", "1\v2", false}, {"FF", "1 2", "1\f2", false},
		{"literal-VT", "1\v2", "1\v2", true}, {"case", "YES", "yes", false},
		{"leading-zero", "1", "01", false}, {"decimal", "1", "1.0", false},
		{"NBSP", "1 2", "1\u00a02", false}, {"unicode", "中文", "中文\n", true},
		{"empty", "", "\t\r\n ", true}, {"NUL", "1 2", "1\x002", false},
	} {
		compare(c.name, []byte(c.ref), []byte(c.out), c.want)
	}
	// Explicitly demonstrate narrower support instead of claiming full wcmp
	// equivalence: invalid UTF-8 and >64 KiB tokens can be accepted upstream.
	for _, out := range [][]byte{{0xfe}, bytes.Repeat([]byte("a"), (1<<16)+1)} {
		write(t, reference, out)
		write(t, output, out)
		if code := child(t, 5*time.Second, binary, input, output, reference); code != 0 {
			t.Fatalf("domain control changed: %d", code)
		}
		if _, err := judging.CompareLibraryCheckerText(out, out, limits); !errors.Is(err, judging.ErrReference) {
			t.Fatalf("domain control not rejected: %v", err)
		}
	}
	write(t, reference, []byte("1"))
	write(t, output, []byte("1\xffignored"))
	if code := child(t, 5*time.Second, binary, input, output, reference); code != 0 {
		t.Fatalf("upstream EOF-sentinel control changed: %d", code)
	}
	if ok, err := judging.CompareLibraryCheckerText([]byte("1"), []byte("1\xffignored"), limits); ok || err != nil {
		t.Fatalf("0xFF must not be accepted: %v %v", ok, err)
	}
	lockHash := sha256.Sum256(lockBytes)
	t.Logf("component=%s lock_sha256=%x cases=%d slice_and_frozen_comparisons=%d answer_bytes=%d max_answer_bytes=%d answers_over_M0=%d limits=%+v; domain exclusions verified; draft validation remains not-run", judging.LibraryCheckerTextVersion, lockHash, len(cases), comparisons, total, maxAnswer, overM0, limits)
}

func checkFrozenComparison(t *testing.T, ref, out string, refSize, outSize int64, limits judging.TextLimits, want bool) {
	t.Helper()
	open := func(path string) *os.File {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	answer := open(ref)
	closeFile := func(f *os.File) {
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}
	defer closeFile(answer)
	actual := open(out)
	defer closeFile(actual)
	got, err := judging.CompareLibraryCheckerTextAt(t.Context(),
		judging.FrozenText{Reader: answer, Size: refSize},
		judging.FrozenText{Reader: actual, Size: outSize}, limits)
	if got != want || err != nil {
		t.Fatalf("frozen site=%v err=%v expected=%v", got, err, want)
	}
}

func checkHash(t *testing.T, raw []byte, want string) {
	t.Helper()
	h := sha256.Sum256(raw)
	if len(want) != 64 || fmt.Sprintf("%x", h) != want {
		t.Fatal("corpus content differs from pinned hash.json")
	}
}
