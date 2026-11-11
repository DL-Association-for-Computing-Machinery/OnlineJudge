package judging

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"unicode/utf8"
)

const textReadBuffer = 16 << 10

// ErrTextRead marks missing, inconsistent or unreadable checker material.
// It is a system failure, not a contestant mismatch.
var ErrTextRead = errors.New("text checker material read failed")

// FrozenText is a read-only, immutable byte span of known size. The caller
// owns the reader and must keep it alive and stable through every pass. No
// reads outside [0, Size) are performed.
type FrozenText struct {
	Reader io.ReaderAt
	Size   int64
}

// ValidateLibraryCheckerReferenceAt checks a frozen reference without loading
// it into memory. Cancellation is checked at reads; ReaderAt itself must
// return promptly or provide its own cancellation.
func ValidateLibraryCheckerReferenceAt(ctx context.Context, reference FrozenText, limits TextLimits) error {
	if !validTextLimits(limits) {
		return ErrTextLimits
	}
	_, err := validateFrozenReference(ctx, reference, limits)
	return err
}

// CompareLibraryCheckerTextAt preserves the slice checker's domain and
// precedence. Both inputs are validated before tokens are compared, so a
// mismatch cannot hide later invalid material. Scratch space is bounded by
// two read buffers and two tokens, independent of the file sizes.
func CompareLibraryCheckerTextAt(ctx context.Context, reference, output FrozenText, limits TextLimits) (bool, error) {
	if !validTextLimits(limits) {
		return false, ErrTextLimits
	}
	ref, err := validateFrozenReference(ctx, reference, limits)
	if err != nil {
		return false, err
	}
	if err := checkFrozen(ctx, output, limits.Bytes, ErrOutput); err != nil {
		return false, err
	}
	out := newTextReader(ctx, output)
	invalid, long, err := scanFrozen(out.buffer, limits.TokenBytes)
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if invalid {
		return false, nil
	}
	if long {
		return false, ErrOutput
	}
	ref.rewind()
	out.rewind()
	a, b := make([]byte, 0, limits.TokenBytes), make([]byte, 0, limits.TokenBytes)
	for {
		a, err = readFrozenToken(ref.buffer, a)
		if err != nil {
			return false, err
		}
		b, err = readFrozenToken(out.buffer, b)
		if err != nil {
			return false, err
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if !bytes.Equal(a, b) {
			return false, nil
		}
		if len(a) == 0 {
			return true, nil
		}
	}
}

func validateFrozenReference(ctx context.Context, text FrozenText, limits TextLimits) (*textReader, error) {
	if err := checkFrozen(ctx, text, limits.Bytes, ErrReference); err != nil {
		return nil, err
	}
	ref := newTextReader(ctx, text)
	invalid, long, err := scanFrozen(ref.buffer, limits.TokenBytes)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if invalid || long {
		return nil, ErrReference
	}
	return ref, nil
}

func checkFrozen(ctx context.Context, text FrozenText, limit int, overflow error) error {
	if ctx == nil {
		return ErrTextRead
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if text.Reader == nil || text.Size < 0 {
		return ErrTextRead
	}
	if text.Size > int64(limit) {
		return overflow
	}
	return nil
}

type textReader struct {
	ctx    context.Context
	text   FrozenText
	offset int64
	buffer *bufio.Reader
}

func newTextReader(ctx context.Context, text FrozenText) *textReader {
	r := &textReader{ctx: ctx, text: text}
	r.buffer = bufio.NewReaderSize(r, textReadBuffer)
	return r
}

func (r *textReader) rewind() {
	r.offset = 0
	r.buffer.Reset(r)
}

func (r *textReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.offset == r.text.Size {
		return 0, io.EOF
	}
	n := int(min(int64(len(p)), r.text.Size-r.offset))
	got, err := r.text.Reader.ReadAt(p[:n], r.offset)
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	if got != n || (err != nil && err != io.EOF) {
		// The caller owns private material. Don't expose paths or lower-level
		// diagnostics through a checker error, but retain cancellation identity.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, errors.Join(ErrTextRead, contextError(err))
		}
		return 0, ErrTextRead
	}
	r.offset += int64(got)
	return got, nil
}

func contextError(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return context.DeadlineExceeded
}

func textRuneSpace(r rune) bool { return r <= 127 && libraryCheckerSpace(byte(r)) }

func scanFrozen(r *bufio.Reader, cap int) (invalid, long bool, err error) {
	var length int
	for {
		ch, width, err := r.ReadRune()
		if err == io.EOF {
			return invalid, long, nil
		}
		if err != nil {
			return false, false, err
		}
		if ch == utf8.RuneError && width == 1 {
			invalid = true
		}
		if textRuneSpace(ch) {
			length = 0
		} else {
			length += width
			long = long || length > cap
		}
	}
}

func readFrozenToken(r *bufio.Reader, token []byte) ([]byte, error) {
	token = token[:0]
	for {
		ch, width, err := r.ReadRune()
		if err == io.EOF {
			return token, nil
		}
		if err != nil {
			return nil, err
		}
		if textRuneSpace(ch) {
			if len(token) != 0 {
				return token, nil
			}
			continue
		}
		if (ch == utf8.RuneError && width == 1) || width > cap(token)-len(token) {
			return nil, ErrTextRead
		}
		token = utf8.AppendRune(token, ch)
	}
}
