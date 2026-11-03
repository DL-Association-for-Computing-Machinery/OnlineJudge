package problem

type diskReferenceAdmission struct {
	path   string
	limits ReferenceLimits
}

func referenceDiskAdmission(sourceID string) (diskReferenceAdmission, error) {
	switch sourceID {
	case "library-checker:unionfind":
		return diskReferenceAdmission{"data_structure/unionfind", UnionFindReferenceLimits()}, nil
	case "library-checker:zalgorithm":
		return diskReferenceAdmission{"string/zalgorithm", ReferenceLimits{SourceBytes: 64 << 10, InputBytes: 1 << 20, AnswerBytes: 4 << 20, TotalBytes: 64 << 20, Cases: 64}}, nil
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
