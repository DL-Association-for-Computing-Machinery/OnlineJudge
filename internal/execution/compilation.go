package execution

import (
	"context"
	"path"
	"strings"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/experiment"
)

// SourceFile supplies private bytes for one sandbox-relative compilation file.
// A caller cannot supply a cache ID or a host path through this capability.
type SourceFile struct {
	Name string `json:"name"`
	Data []byte `json:"-"`
}

// CompilationSandbox is an optional bounded capability for frozen dependencies.
// Legacy Sandbox implementations remain valid for single-source profiles.
type CompilationSandbox interface {
	CompileFiles(context.Context, experiment.ExecutionProfile, []SourceFile) (Observation, error)
}

func cachedProgramName(p experiment.ExecutionProfile) string {
	if experiment.IsJava21Profile(p) {
		return "main.jar"
	}
	if experiment.IsPython3Profile(p) {
		return "main.pyc"
	}
	return "main"
}

func (g *CachedGoJudge) prepareCompilation(source string, files []SourceFile, multiple bool) ([]SourceFile, string, bool) {
	if !multiple {
		if g.compilationEntry != "" || int64(len(source)) > g.limits.SourceBytes {
			return nil, "", false
		}
		name := experiment.SingleSourceName(g.runtime)
		return []SourceFile{{Name: name, Data: []byte(source)}}, name, true
	}
	if !g.reference || g.compilationEntry == "" || len(files) > 5 {
		return nil, "", false
	}
	files = append([]SourceFile(nil), files...)
	if !validCompilation(files, g.compilationEntry, g.limits.SourceBytes) {
		return nil, "", false
	}
	// Detach the fully prechecked bounded bundle before the first upload.
	for i := range files {
		files[i].Data = append([]byte(nil), files[i].Data...)
	}
	return files, g.compilationEntry, true
}

func (g *CachedGoJudge) uploadCompilation(ctx context.Context, files []SourceFile, entry string) (map[string]any, []*cachedObject, error) {
	copyIn := make(map[string]any, len(files))
	var objects []*cachedObject
	for _, file := range files {
		role := "dependency"
		if file.Name == entry {
			role = "source"
		}
		object, err := g.upload(ctx, file.Data, role)
		if object != nil {
			objects = append(objects, object)
		}
		if err != nil {
			return nil, objects, err
		}
		copyIn[file.Name] = map[string]any{"fileId": object.id}
	}
	return copyIn, objects, nil
}

func validCompilation(files []SourceFile, entry string, maximum int64) bool {
	if len(files) < 1 || len(files) > 5 || entry == "" {
		return false
	}
	seen := map[string]bool{}
	var total int64
	for _, f := range files {
		name := f.Name
		if len(name) == 0 || len(name) > 128 || path.IsAbs(name) || path.Clean(name) != name || strings.Contains(name, "\\") {
			return false
		}
		for _, component := range strings.Split(name, "/") {
			if component == "" || component == "." || component == ".." {
				return false
			}
			for _, c := range component {
				allowed := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.'
				if !allowed {
					return false
				}
			}
		}
		canonical := strings.ToLower(name)
		for _, reserved := range []string{"main", "stdin", "stdout", "stderr"} {
			if canonical == reserved || strings.HasPrefix(canonical, reserved+"/") {
				return false
			}
		}
		for old := range seen {
			if canonical == old || strings.HasPrefix(canonical, old+"/") || strings.HasPrefix(old, canonical+"/") {
				return false
			}
		}
		if int64(len(f.Data)) > maximum-total {
			return false
		}
		total += int64(len(f.Data))
		seen[canonical] = true
	}
	// Entry spelling is exact even though collision checks also reject case aliases.
	for _, f := range files {
		if f.Name == entry {
			return true
		}
	}
	return false
}
