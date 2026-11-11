package problem

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"

const (
	localReferenceSourceLock = "fe2722edade748228c9add9c54704c09861fa6be649b5cdf59e1f214b5ee9617"
	zAlgorithmLocalVersion   = "965239246cbdbd63f1641cfcf551d9c8f3a65cd558e194668ad845cac1e39711"
	aPlusBLocalVersion       = "74c7797f5484daf9df01b1be2e5a537eba7b05b1ef20efc08110c61e3c1490fd"
	unionFindLocalVersion    = "42e62825925bf727c394f3435c2b50f850758576c392d082fb628f7219e4f07d"
	staticRMQLocalVersion    = "194da8b53c88aa634cb5c030c684a355a59b224a00c7d74422bc35b333d3a5f0"
)

const associativeArrayLocalVersion = "4d6dd8e3fe97a0e1ec6585ff538219d811f660bdfff7cc2172bf6092cf858928"

type localReferenceRevision struct {
	label, sourceID            string
	upstream                   int
	source, definition, hashes materialstore.Object
	suffix                     []ReferenceCase
}

// Only reviewed content versions can append local cases. Both preparation and
// Worker transport still check the ordered upstream catalogue independently.
var localReferenceRevisions = map[string]localReferenceRevision{
	zAlgorithmLocalVersion: {
		label: "oj-local:zalgorithm-scale-v1", sourceID: "library-checker:zalgorithm", upstream: 29,
		source:     materialstore.Object{SHA256: "9f7168d28138fb61d7706b5b899bb0ae21503a848428fc7d7551c5a8507ff8d7", Size: 969},
		definition: materialstore.Object{SHA256: "75d80d297a4f12c13c828b289a4e5795d8573d1a668ad07e4f19c81dcc54fc4c", Size: 499},
		hashes:     materialstore.Object{SHA256: "45ec72478cb3d175a65be662df6dc2e2ada46f6fd146f796dd07ea3174a8cf84", Size: 5099},
		suffix: []ReferenceCase{
			{
				ID:     "case-0030",
				Input:  materialstore.Object{SHA256: "87428fc522803d31065e7bce3cf03fe475096631e5e07bbd7a0fde60c4cf25c7", Size: 2},
				Answer: materialstore.Object{SHA256: "4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865", Size: 2},
			},
			{
				ID:     "case-0031",
				Input:  materialstore.Object{SHA256: "069af87ff42316e1bbe6cc65d3ed71a71cdcf9cc972257c93a7d934b435c8ad2", Size: 500001},
				Answer: materialstore.Object{SHA256: "9a3768bb91a7c39e5777767f091605ebb81cac9a047140ba7832bc5f067ab434", Size: 3388895},
			},
		},
	},
	aPlusBLocalVersion: {
		label: "oj-local:aplusb-boundaries-v1", sourceID: "library-checker:aplusb", upstream: 12,
		source:     materialstore.Object{SHA256: "5cd36270bccae9a25f48f29df5187569a3919f26dc2440485916c43cec07a70f", Size: 387},
		definition: materialstore.Object{SHA256: "402dded4217583a68f9fdd800909f2c3f915e0698a5b392654e9e753d03cc047", Size: 359},
		hashes:     materialstore.Object{SHA256: "c328b044bc45ce3a0eef37c9e9e8bb6791409bc8bfedf944e106f3aadd7fb20c", Size: 2082},
		suffix: []ReferenceCase{
			{
				ID:     "case-0013",
				Input:  materialstore.Object{SHA256: "0ccdb5a77ba5bf7687f2565a8ed97dfb9c1af45503c496fb646312239fab5101", Size: 4},
				Answer: materialstore.Object{SHA256: "9a271f2a916b0b6ee6cecb2426f0b3206ef074578be55d9bc94f6f3fe3ab86aa", Size: 2},
			},
			{
				ID:     "case-0014",
				Input:  materialstore.Object{SHA256: "a79122992d53d358e6bbbbb98883d64fa0c15df3bcb08ff7b65a0580870af424", Size: 4},
				Answer: materialstore.Object{SHA256: "4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865", Size: 2},
			},
			{
				ID:     "case-0015",
				Input:  materialstore.Object{SHA256: "f4a8ae8e74ddfb896a256de4e3099911dcaa6a9302591713898069b0bcd6e3d7", Size: 4},
				Answer: materialstore.Object{SHA256: "4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865", Size: 2},
			},
			{
				ID:     "case-0016",
				Input:  materialstore.Object{SHA256: "094f53851d789a7dfbebbbd19875d6dff79aeb9b1dbbadefc1f5bab77d597ff0", Size: 21},
				Answer: materialstore.Object{SHA256: "873cf3b1e4f23454700801f646b785edf54fe87150bbda8c8c0a7f657fc01979", Size: 11},
			},
			{
				ID:     "case-0017",
				Input:  materialstore.Object{SHA256: "4d0b3116b1f3552d37cd16d57c533acc42ae47e198d201c0173bd7979528b1e2", Size: 21},
				Answer: materialstore.Object{SHA256: "873cf3b1e4f23454700801f646b785edf54fe87150bbda8c8c0a7f657fc01979", Size: 11},
			},
		},
	},
	staticRMQLocalVersion: {
		label: "oj-local:staticrmq-single-query-max-v1", sourceID: "library-checker:staticrmq", upstream: 27,
		source:     materialstore.Object{SHA256: "a5586c48fa83887a773876dc5377ab6c5f7b59264051d6484ea7712ca5aafb3c", Size: 1890},
		definition: materialstore.Object{SHA256: "6efb4f756209387d8046d01b25cd927151205f2b5b074fb171b5c8fc983263cd", Size: 488},
		hashes:     materialstore.Object{SHA256: "18e6c6649f5a76cb68f0ddf67c1273b43d032b1c17ebbd8a5c002b90b5ab14bb", Size: 4817},
		suffix: []ReferenceCase{
			{
				ID:     "case-0028",
				Input:  materialstore.Object{SHA256: "30ad661dba75866c8b8ae4977d14abd8081b2cdf190133af4eb4d027cc263fe0", Size: 34},
				Answer: materialstore.Object{SHA256: "8bd5a23c3aef1243c78bac155b39b9d57a43d8c49665ccf6f42acf2992a94afa", Size: 11},
			},
		},
	},
	unionFindLocalVersion: {
		label: "oj-local:unionfind-singleton-v1", sourceID: "library-checker:unionfind", upstream: 18,
		source:     materialstore.Object{SHA256: "c17d0ae17487b004935e4f9bb4d43738d627bf68f63470239f170e2e9ca42024", Size: 1140},
		definition: materialstore.Object{SHA256: "848ea059fdd5dd16444ea3323882ca7bbae64a0f88090506dc1522eb6e15a796", Size: 359},
		hashes:     materialstore.Object{SHA256: "a5c718fe24e2a83f31aa0902c065d31229cf90e503128354d30bc4c568233a41", Size: 3126},
		suffix: []ReferenceCase{
			{
				ID:     "case-0019",
				Input:  materialstore.Object{SHA256: "6e1e9445fd0bd0736d00d1cafb78d3c101ae577a258c5178efd9e8909f20ddb8", Size: 10},
				Answer: materialstore.Object{SHA256: "4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865", Size: 2},
			},
			{
				ID:     "case-0020",
				Input:  materialstore.Object{SHA256: "5b12ce47a7cd1d0c27176fd80881c7d8676b680b86626b9df12d4e78095744e3", Size: 10},
				Answer: materialstore.Object{SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", Size: 0},
			},
		},
	},
	associativeArrayLocalVersion: {
		label: "oj-local:associative-array-boundaries-v1", sourceID: "library-checker:associative_array", upstream: 20,
		source:     materialstore.Object{SHA256: "8f3c2b6f6e2e489b1bf385e1363a27e80a247e98f47a17fae947abb70fff2da0", Size: 474},
		definition: materialstore.Object{SHA256: "235aaecdbf2574f548d50a0327dc38690bf7779b2e64aa0dafdf8a717c81fbfd", Size: 866},
		hashes:     materialstore.Object{SHA256: "b9fc7684c0215fd0adbc53411a84addf42c4189f84004c6e0709eb57890677d7", Size: 3698},
		suffix: []ReferenceCase{
			{
				ID:     "case-0021",
				Input:  materialstore.Object{SHA256: "c8d0143328b6f68a7635e5d2153ec90de750933ec6bff70fb77bbfd997dacf5f", Size: 24},
				Answer: materialstore.Object{SHA256: "9a271f2a916b0b6ee6cecb2426f0b3206ef074578be55d9bc94f6f3fe3ab86aa", Size: 2},
			},
			{
				ID:     "case-0022",
				Input:  materialstore.Object{SHA256: "0b82b4520934c1d9d01fc93245603aed7a521500b7b8e1eb1262a2d9035f0bd2", Size: 66},
				Answer: materialstore.Object{SHA256: "b8650fe462b5b738b2bb69af1d3c3f16974db285c79025cf1b98652763ff509a", Size: 20},
			},
		},
	},
}

func (r localReferenceRevision) matches(sourceID, lock string, points int) bool {
	return sourceID == r.sourceID && lock == localReferenceSourceLock && points == r.upstream+len(r.suffix)
}

func validDocumentCaseMapping(d document) bool {
	revision, known := localReferenceRevisions[d.Version]
	if !known {
		return d.LocalData == "" && len(d.Provenance.Mapping) == len(d.TestCases)
	}
	if d.LocalData != revision.label || !revision.matches(d.Provenance.SourceID, d.Provenance.SourceLockSHA256, len(d.TestCases)) ||
		len(d.Provenance.Mapping) != revision.upstream {
		return false
	}
	for i, pinned := range revision.suffix {
		c := d.TestCases[revision.upstream+i]
		if c.Sample || c.ID != pinned.ID || c.Input.object() != pinned.Input || c.Answer.object() != pinned.Answer {
			return false
		}
	}
	return true
}

// upstreamPlanSize keeps the original encoding and full-catalogue behavior for
// ordinary versions. A reviewed local version must have its exact suffix.
func upstreamPlanSize(p ReferencePlan) (int, bool) {
	revision, known := localReferenceRevisions[p.Version]
	if !known {
		return len(p.Cases), true
	}
	if !revision.matches(p.SourceID, p.SourceLockSHA256, len(p.Cases)) ||
		p.Source != revision.source || p.Definition != revision.definition || p.Hashes != revision.hashes {
		return 0, false
	}
	for i, pinned := range revision.suffix {
		c := p.Cases[revision.upstream+i]
		if c.ID != pinned.ID || c.Input != pinned.Input || c.Answer != pinned.Answer {
			return 0, false
		}
	}
	return revision.upstream, true
}
