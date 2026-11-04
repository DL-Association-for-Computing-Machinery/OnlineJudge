package problem

type diskReferenceAdmission struct {
	path         string
	limits       ReferenceLimits
	entry        string
	dependencies []referenceDependencyAdmission
}

type referenceDependencyAdmission struct{ original, target string }

func referenceDiskAdmission(sourceID string) (diskReferenceAdmission, error) {
	switch sourceID {
	case "library-checker:unionfind":
		return diskReferenceAdmission{path: "data_structure/unionfind", limits: UnionFindReferenceLimits()}, nil
	case "library-checker:zalgorithm":
		return diskReferenceAdmission{path: "string/zalgorithm", limits: ReferenceLimits{SourceBytes: 64 << 10, InputBytes: 1 << 20, AnswerBytes: 4 << 20, TotalBytes: 64 << 20, Cases: 64}}, nil
	case "library-checker:suffixarray":
		return diskReferenceAdmission{path: "string/suffixarray", limits: ReferenceLimits{SourceBytes: 64 << 10, InputBytes: 1 << 20, AnswerBytes: 4 << 20, TotalBytes: 128 << 20, Cases: 64}}, nil
	case "library-checker:static_range_sum":
		return diskReferenceAdmission{path: "data_structure/static_range_sum", limits: ReferenceLimits{SourceBytes: 64 << 10, InputBytes: 16 << 20, AnswerBytes: 8 << 20, TotalBytes: 256 << 20, Cases: 64}}, nil
	case "library-checker:primality_test":
		return diskReferenceAdmission{path: "number_theory/primality_test", limits: ReferenceLimits{SourceBytes: 64 << 10, InputBytes: 2 << 20, AnswerBytes: 512 << 10, TotalBytes: 32 << 20, Cases: 64}, entry: "sol/correct.cpp", dependencies: []referenceDependencyAdmission{
			{"source/number_theory/primality_test/lib/prime.hpp", "lib/prime.hpp"},
			{"source/number_theory/primality_test/lib/montgomery.hpp", "lib/montgomery.hpp"},
			{"source/number_theory/primality_test/lib/util.hpp", "lib/util.hpp"},
			{"source/common/random.h", "common/random.h"},
		}}, nil
	default:
		return diskReferenceAdmission{}, ErrReferenceUnsupported
	}
}

// DiskReferenceLimits returns only fixed admitted complete-source budgets.
// Unknown sources cannot borrow another source's material or runtime profile.
func DiskReferenceLimits(sourceID string) (ReferenceLimits, error) {
	a, err := referenceDiskAdmission(sourceID)
	return a.limits, err
}
