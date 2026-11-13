package experiment

import (
	"errors"

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
