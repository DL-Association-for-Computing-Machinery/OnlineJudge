package problemvalidation

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/judging"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materials"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
)

func readLocalDecimal(ctx context.Context, prepared *problem.Prepared) (*judging.DecimalPolicy, error) {
	on, err := prepared.DeclaresLocalDecimal()
	if err != nil || !on {
		return nil, err
	}
	raw, err := prepared.ReadLocalTextQualityFile(ctx, "quality/decimal.json", 4<<10)
	if err != nil {
		return nil, ErrCheckerCandidateBinding
	}
	var policy judging.DecimalPolicy
	if json.Unmarshal(raw, &policy) != nil || !policy.Valid() {
		return nil, ErrCheckerCandidateBinding
	}
	canonical, err := json.Marshal(policy)
	if err != nil || !bytes.Equal(raw, append(canonical, '\n')) {
		return nil, ErrCheckerCandidateBinding
	}
	return &policy, nil
}

func bindLocalDecimal(m *materials.TextTransportManifest, policy *judging.DecimalPolicy) {
	m.Checker, m.Decimal, m.TokenBytes = judging.DecimalVersion, policy, 0
	m.JudgeSHA256 = localDecimalDigest(m.JudgeSHA256, policy)
	m.Version = "local-text-" + m.JudgeSHA256
}

func localDecimalDigest(original string, policy *judging.DecimalPolicy) string {
	return digest(struct {
		Original string
		Version  string
		Policy   *judging.DecimalPolicy
	}{original, judging.DecimalVersion, policy})
}
