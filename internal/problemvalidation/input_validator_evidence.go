package problemvalidation

import "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"

// InputValidatorEvidence summarizes the admitted historical input checks.
// It does not establish checker admission, coverage, or production resources.
type InputValidatorEvidence struct {
	ReportSHA256 string `json:"report_sha256"`
	Inputs       int    `json:"inputs"`
	Controls     int    `json:"controls"`
}

const admittedInputValidatorReport = "cc6907675c87d93e6c0dd08fc637d13bcd34ff6b4ba9e497fbce4c6681578a9d"
const admittedInputValidatorLock = "fe2722edade748228c9add9c54704c09861fa6be649b5cdf59e1f214b5ee9617"

type reviewedV6Binding struct {
	version, revision, plan string
	count                   int
	oracleCases             int
}

// These bindings are for the reviewed Library Checker v6 run only. The full
// import revision binds the source, imported inputs and report itself. The
// plan digest also pins the reference program and ordered review cases.
var reviewedV6Bindings = map[string]reviewedV6Binding{
	"library-checker:aplusb": {
		version:  "87ebb9680aa3abe190b61d4a5ccac1dc83e48b29656364e445e36fabe7467cc9",
		revision: "d19c436bc1631cb2e42b38f0036de45c100ea2f76c953df502b960287c248dd5",
		plan:     "4632b8970118f8a75ac39857d31d2afb8244683892b9edc49357f9f5aefeba2a", count: 12, oracleCases: 25,
	},
	"library-checker:associative_array": {
		version:  "cb50459ffb3537de06ff6b931ea9047842d92a9420c39e37f9fb3efaa73e1271",
		revision: "fab3cca3aae9233521b075dccf6043efcd2b5c418b250692e917c8f5dc40733b",
		plan:     "3c7a51cec70f93d36f083980e1d59308fea2a58afd7d5589522f12f955743477", count: 20, oracleCases: 4,
	},
	"library-checker:convolution_mod": {
		version:  "7a1079bd0d8be8d2cb57619780d4f519de096fd3bb9655eaa37a9bfe49e19e80",
		revision: "e1c721627cd97dec382d90bfe559e766b7f3811d9257ce89946404befe009975",
		plan:     "8a0d5909fce14750a809d59cece1bc0dede3f41c8dce6fba0f1ad92e371471d9", count: 53,
	},
	"library-checker:point_add_range_sum": {
		version:  "e1fb107ff818b642bb5976ff35e7aaff5e8bb2ceb333972cb3acaf1256476bb1",
		revision: "f87182dcfd865eb040a18c73db8bc69d4ad2388efffc67ff38f04e5ed4a912a9",
		plan:     "0096ebc66968a23ac3bbb102799c59c61d9ea369f2f6613082641234930a1931", count: 21, oracleCases: 8,
	},
	"library-checker:primality_test": {
		version:  "5447e801527926aa5e4b7da82101b30acaa366fdce105123c83384888fcd17af",
		revision: "54c2b27b169935f6f52f64a651f37b6495a2f87af56af446f888739b8a26115f",
		plan:     "bf9cf266823e39cf3db437bbe9a5d5cf19fbc31ad56b8c9ffa341d1df5f63f2c", count: 13, oracleCases: 5,
	},
	"library-checker:static_range_sum": {
		version:  "4d3adee969250c1834a61d59b456649f0c2cd0445902ad386b4a6f3e351a2f93",
		revision: "a93ddf11ed951ff9f43de034f53968f3b709e0cf763fa69bd13c8e3c05bb6496",
		plan:     "80d63d06041473a349fe3cf0986cee2a15ab0badfb1a6d81efc5079347fa3f4d", count: 11, oracleCases: 8,
	},
	"library-checker:staticrmq": {
		version:  "c5eef70e8f853a4525257371c895f01bd2ac40222d12cc72b83f3782a712186c",
		revision: "016007d87f12c525a4965b345505ef48c3721b0d600b9a4346ac0879198e7a49",
		plan:     "db9f1d7fb105d7d5be0f9deff4c3d1e7cad4a9ce8b8cc71e1baaf31fdde5e1c1", count: 27, oracleCases: 8,
	},
	"library-checker:suffixarray": {
		version:  "b7622607e0db239445f7b75b14bc9edc10b7992f1c6b6254fd4f784aa5cb0d97",
		revision: "c7a2c14e76fb9a64d9f6183f167effb397d1758d902f12c2d9d46e12a63595da",
		plan:     "1f3851785ae5ffdbea93948468a1ec8c31d087941f4ac9746dac891efe43efd0", count: 50, oracleCases: 62,
	},
	"library-checker:unionfind": {
		version:  "fd871a02594fcd663790cd0530153df2ad18bf95110559ce552a121c34705bed",
		revision: "14b95a5dcfe72e5be314ed8b7739e1330057a2bfbf85cdc3bfac30904fb557c7",
		plan:     "d64b9c33fd865c0433ba7b932ccfd6c9184793512a271c9a618b510a05b1d37d", count: 18, oracleCases: 10,
	},
	"library-checker:zalgorithm": {
		version:  "151da5a28d0bd733fe4837547ec909137a36b0e532409c93dfd7bf3c2a7e6b41",
		revision: "e10aa8033bda67a3d047c07df73e991c0e52d8567a406a3eb956336ccb55733d",
		plan:     "6745f35c02ba6536c033a81b74bbc643193c4dd76e34db0dcbe348b3c49fb4ea", count: 29, oracleCases: 62,
	},
}

// AdmittedInputValidator returns evidence only for the exact reviewed binding.
// Callers must first verify the identity and plan in trusted stored evidence.
func AdmittedInputValidator(identity problem.ImportIdentity, planSHA256 string) (InputValidatorEvidence, bool) {
	row, ok := admittedV6Binding(identity, planSHA256)
	if !ok {
		return InputValidatorEvidence{}, false
	}
	return InputValidatorEvidence{ReportSHA256: admittedInputValidatorReport, Inputs: row.count, Controls: 4}, true
}

func admittedV6Binding(identity problem.ImportIdentity, planSHA256 string) (reviewedV6Binding, bool) {
	row, ok := reviewedV6Bindings[identity.SourceID]
	if !ok || identity.SourceLockSHA256 != admittedInputValidatorLock || identity.Version != row.version ||
		identity.ImportRevision != row.revision || planSHA256 != row.plan {
		return reviewedV6Binding{}, false
	}
	return row, true
}
