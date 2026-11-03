package experiment

import (
	"reflect"
	"testing"
)

func TestStaticRMQTierChangesOnlyIdentityAndCumulativePayload(t *testing.T) {
	m, _ := ResolveReferenceTier(ReferenceMedium)
	c, err := ResolveReferenceTier(ReferenceStaticRMQ)
	if err != nil || ReferenceStaticRMQ != 5 || c.Entry != "" || c.Runtime.ID != "p4-staticrmq-gnu17-desktop-arm64-v1" || c.Files.ID != "p4-gojudge-cached-staticrmq-v1" || c.Files.PayloadBytes != 512<<20 {
		t.Fatal("staticrmq configuration identity", err)
	}
	c.Runtime.ID, c.Files.ID, c.Files.PayloadBytes = m.Runtime.ID, m.Files.ID, m.Files.PayloadBytes
	if !reflect.DeepEqual(c, m) {
		t.Fatal("staticrmq changed execution, outer isolation or live-file bounds")
	}
	c.Runtime.CompileArgs[0] = "changed"
	again, _ := ResolveReferenceTier(ReferenceStaticRMQ)
	if again.Runtime.CompileArgs[0] != "/usr/local/bin/g++" {
		t.Fatal("mutable configuration escaped")
	}
}
