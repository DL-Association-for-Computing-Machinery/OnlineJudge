package experiment

import (
	"errors"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/checkedfile"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
)

// LoadAPlusBFixture applies the same explicit local profile configuration for
// registration, controller and Worker. The transport manifest cannot choose it.
func LoadAPlusBFixture(getenv func(string) string) (materials.APlusBManifest, WorkerFixture, error) {
	var tier ReferenceTier
	switch getenv("OJ_APLUSB_RUNTIME") {
	case "":
		tier = ReferenceSmall
	case "ubuntu-arm64":
		tier = ReferenceAPlusBUbuntu
	default:
		return materials.APlusBManifest{}, WorkerFixture{}, errors.New("unsupported A+B runtime setting")
	}
	m, raw, enabled, err := materials.LoadAPlusB(getenv)
	if err != nil {
		return materials.APlusBManifest{}, WorkerFixture{}, err
	}
	if !enabled {
		if tier != ReferenceSmall {
			return materials.APlusBManifest{}, WorkerFixture{}, errors.New("the Ubuntu A+B runtime requires a pinned material manifest")
		}
		return materials.APlusBManifest{}, WorkerFixture{}, nil
	}
	f, err := aplusbFixtureForTier(raw, getenv("OJ_APLUSB_MANIFEST_SHA256"), tier)
	return m, f, err
}

// LoadCachedFixture accepts one explicit legacy or local text queue. Local
// text reuses the pinned Ubuntu sandbox; it cannot supply a runtime profile.
func LoadCachedFixture(getenv func(string) string) (materials.TextTransportManifest, WorkerFixture, error) {
	path, pin := getenv("OJ_LOCAL_TEXT_MANIFEST_FILE"), getenv("OJ_LOCAL_TEXT_MANIFEST_SHA256")
	if path == "" && pin == "" {
		return LoadAPlusBFixture(getenv)
	}
	var m materials.TextTransportManifest
	var f WorkerFixture
	if getenv("OJ_SANDBOX_EXPERIMENT") != "1" || getenv("OJ_EXPERIMENT_MATERIAL_VERSION") != "" ||
		getenv("OJ_APLUSB_MANIFEST_FILE") != "" || getenv("OJ_APLUSB_MANIFEST_SHA256") != "" || getenv("OJ_APLUSB_RUNTIME") != "" {
		return m, f, materials.ErrUnavailable
	}
	raw, err := checkedfile.ReadSecret(path, 16384)
	if err != nil {
		return m, f, err
	}
	m, err = materials.ParseLocalText(raw, pin)
	runtime := APlusBRuntimeSHA256ForTier(ReferenceAPlusBUbuntu)
	if m.Checker == materials.LocalSPJChecker {
		runtime = LocalSPJRuntimeSHA256()
	}
	if err != nil || m.RuntimeSHA256 != runtime {
		return m, f, materials.ErrUnavailable
	}
	return m, WorkerFixture{aplusb: string(raw), aplusbVersion: m.Version, aplusbTier: ReferenceAPlusBUbuntu, localText: true, checkerSHA: m.CheckerArtifact}, nil
}
