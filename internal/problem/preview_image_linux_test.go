//go:build linux

package problem

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/session/redisstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/tests/problemfixture"
)

func TestPreviewImagesAreExplicitVersionBoundAndReencoded(t *testing.T) {
	raw := problemfixture.PNG(t, 80, 40)
	// Complete decoding and re-encoding must remove arbitrary trailing content.
	raw = append(raw, []byte("private trailing metadata")...)
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	var jpegBytes bytes.Buffer
	if err := jpeg.Encode(&jpegBytes, decoded, nil); err != nil {
		t.Fatal(err)
	}
	f := newPreviewFixture(t, func(b *problemfixture.Bundle) {
		b.AddStatementImage(t, "figures/a.png", raw)
		b.AddStatementImage(t, "b.jpg", jpegBytes.Bytes())
		b.AddStatementImage(t, "unselected.png", problemfixture.PNG(t, 10, 10))
		delete(b.Document["statement"].(map[string]any)["images"].(map[string]any), "unselected.png")
	})
	f.importer.sources = nil // A new candidate cannot alter old persisted assets.
	got, err := f.importer.Preview(t.Context(), f.sessions.credential, previewID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Images) != 2 || got.Images[0].Source != "b.jpg" || got.Images[1].Source != "figures/a.png" || len(f.materials.opened) != 5 || f.repository.gets != 2 {
		t.Fatal("image roles, order or final authorization changed")
	}
	for _, asset := range got.Images {
		if asset.MediaType != "image/png" || asset.Width != 80 || asset.Height != 40 || bytes.Contains(asset.Data, []byte("private trailing")) {
			t.Fatal("source payload escaped normalization")
		}
		if _, err := png.Decode(bytes.NewReader(asset.Data)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStatementImageManifestRejectsAliasesAndNonLocalObjects(t *testing.T) {
	for _, name := range []string{"../a.png", "./a.png", "/a.png", "a//b.png", "a/../b.png", "https:a.png", "a%2eb.png", "a.png?x", "a#b.png", "a\\b.png", "a b.png", ".hidden.png", "a.svg", "a.PNG"} {
		if validImageSource(name) {
			t.Fatalf("accepted alias %q", name)
		}
	}
	for _, kind := range []string{"foreign-original", "unbound", "empty", "too-many", "oversize", "null"} {
		t.Run(kind, func(t *testing.T) {
			b := problemfixture.New(t)
			b.AddStatementImage(t, "a.png", problemfixture.PNG(t, 1, 1))
			s := b.Document["statement"].(map[string]any)
			images := s["images"].(map[string]any)
			switch kind {
			case "foreign-original":
				images["a.png"] = b.Document["test_cases"].([]any)[0].(map[string]any)["input"]
			case "unbound":
				images["b.png"] = images["a.png"]
			case "empty":
				s["images"] = map[string]any{}
			case "null":
				s["images"] = nil
			case "too-many":
				for n := 0; n < 17; n++ {
					images[strings.Repeat("a", n+2)+".png"] = images["a.png"]
				}
			case "oversize":
				images["a.png"].(map[string]any)["size"] = MaxPreviewImageBytes + 1
			}
			b.Write(t)
			if p, err := LoadBundle(b.Directory, "library-checker:aplusb", b.Lock); err == nil {
				_ = p.Close()
				t.Fatal("accepted invalid image manifest")
			}
		})
	}
}

func TestImageFailuresReturnNoPartialPreview(t *testing.T) {
	for _, kind := range []string{"corrupt", "truncated", "wrong-format", "dimensions", "pixels", "total-pixels", "raw-total", "missing-role", "logout", "role-revoked", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			f := newPreviewFixture(t, func(b *problemfixture.Bundle) {
				raw := problemfixture.PNG(t, 32, 16)
				switch kind {
				case "corrupt":
					raw[len(raw)-16] ^= 0xff
				case "truncated":
					raw = raw[:len(raw)/2]
				case "wrong-format":
					raw = []byte("<svg xmlns='http://www.w3.org/2000/svg'/>")
				case "dimensions", "pixels":
					// A real, highly compressible raster stays inside encoded budget.
					w, h := 4097, 1
					if kind == "pixels" {
						w, h = 2001, 2000
					}
					var out bytes.Buffer
					if err := png.Encode(&out, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
						t.Fatal(err)
					}
					raw = out.Bytes()
				}
				b.AddStatementImage(t, "a.png", raw)
				if kind == "total-pixels" {
					var out bytes.Buffer
					if err := png.Encode(&out, image.NewGray(image.Rect(0, 0, 2000, 2000))); err != nil {
						t.Fatal(err)
					}
					for _, name := range []string{"a.png", "b.png", "c.png"} {
						b.AddStatementImage(t, name, out.Bytes())
					}
				}
				if kind == "raw-total" {
					for _, name := range []string{"a.png", "b.png", "c.png", "d.png"} {
						b.AddStatementImage(t, name, append(raw, make([]byte, int(MaxPreviewImageBytes)-len(raw))...))
					}
				}
			})
			want := error(ErrStore)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch kind {
			case "missing-role":
				for n := range f.repository.draft.References {
					if strings.HasSuffix(f.repository.draft.References[n].Name, "/a.png") {
						f.repository.draft.References[n].Object.SHA256 = strings.Repeat("0", 64)
					}
				}
			case "logout", "role-revoked", "cancel":
				f.materials.onOpen = func() {
					if len(f.materials.opened) != 4 {
						return
					}
					switch kind {
					case "logout":
						f.sessions.err = redisstore.ErrStale
					case "role-revoked":
						f.repository.final = identity.ErrForbidden
					case "cancel":
						cancel()
					}
				}
				if kind == "logout" {
					want = redisstore.ErrStale
				}
				if kind == "role-revoked" {
					want = identity.ErrForbidden
				}
				if kind == "cancel" {
					want = context.Canceled
				}
			}
			got, err := f.importer.Preview(ctx, f.sessions.credential, previewID)
			if !errors.Is(err, want) || !reflect.DeepEqual(got, Preview{}) {
				t.Fatalf("%s leaked partial preview: %v", kind, err)
			}
			if (kind == "missing-role" || kind == "raw-total") && len(f.materials.opened) != 0 {
				t.Fatal("image role/raw budget checked after material I/O")
			}
			if err := f.local.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestImageOutputWriterNeverTruncatesSuccessfully(t *testing.T) {
	var out imageBuffer
	if _, err := out.Write(make([]byte, MaxPreviewImageBytes)); err != nil {
		t.Fatal(err)
	}
	if n, err := out.Write([]byte{1}); n != 0 || !errors.Is(err, ErrStore) || int64(out.Len()) != MaxPreviewImageBytes {
		t.Fatal("output budget silently truncated")
	}
}

func TestReencodingEnforcesIndividualAndAggregateOutputBudgets(t *testing.T) {
	for _, size := range []int{192, 384} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			noise := image.NewNRGBA(image.Rect(0, 0, size, size))
			random := rand.New(rand.NewPCG(7, 13))
			for n := range noise.Pix {
				noise.Pix[n] = byte(random.Uint32())
				if n%4 == 3 {
					noise.Pix[n] = 255
				}
			}
			var original bytes.Buffer
			if err := jpeg.Encode(&original, noise, &jpeg.Options{Quality: 40}); err != nil {
				t.Fatal(err)
			}
			decoded, err := jpeg.Decode(bytes.NewReader(original.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			var normalized bytes.Buffer
			if err := png.Encode(&normalized, decoded); err != nil {
				t.Fatal(err)
			}
			if original.Len() >= int(MaxPreviewImageBytes) {
				t.Fatal("fixture exceeds source admission budget")
			}
			count := 1
			if size == 192 {
				if normalized.Len() > int(MaxPreviewImageBytes) {
					t.Fatal("aggregate fixture exceeds individual output budget")
				}
				count = int(MaxPreviewBytes)/normalized.Len() + 1
			} else if normalized.Len() <= int(MaxPreviewImageBytes) {
				t.Fatal("individual fixture must expand past output budget")
			}
			f := newPreviewFixture(t, func(b *problemfixture.Bundle) {
				for n := 0; n < count; n++ {
					b.AddStatementImage(t, strings.Repeat("a", n+1)+".jpg", original.Bytes())
				}
			})
			got, err := f.importer.Preview(t.Context(), f.sessions.credential, previewID)
			if !errors.Is(err, ErrStore) || !reflect.DeepEqual(got, Preview{}) {
				t.Fatalf("oversize normalized output escaped: %v", err)
			}
		})
	}
}
