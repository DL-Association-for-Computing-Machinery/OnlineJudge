package execution

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"
)

type cachedObject struct {
	id, role string
	size     int64
	released bool
}

func (g *CachedGoJudge) acquire(ctx context.Context, cleanup bool) error {
	if g == nil || ctx == nil {
		return errors.New("cached sandbox unavailable")
	}
	if !g.busy.CompareAndSwap(false, true) {
		return ErrBusy
	}
	if err := ctx.Err(); err != nil {
		g.failed = true
		g.busy.Store(false)
		return err
	}
	if g.failed && !cleanup {
		g.busy.Store(false)
		return errors.New("cached sandbox fenced")
	}
	return nil
}

func (g *CachedGoJudge) finish(ctx context.Context, err error) error {
	err = errors.Join(err, ctx.Err())
	if err != nil {
		g.failed = true
	}
	g.busy.Store(false)
	return err
}

func (g *CachedGoJudge) step(payload int64, cleanup bool) error {
	needed := 1
	if !cleanup {
		needed += g.liveObjects
	} // reserve each existing object's future DELETE
	if payload < 0 || payload > g.limits.PayloadBytes-g.totalBytes || g.operations+needed > g.limits.Operations {
		return withFailureCode(errors.New("cached operation budget exhausted"), "budget")
	}
	g.operations++
	g.totalBytes += payload
	return nil
}

func (g *CachedGoJudge) reserve(size int64, role string) (*cachedObject, error) {
	if size < 0 || size > g.limits.FileBytes || size > g.limits.LiveBytes-g.liveBytes || g.liveObjects >= g.limits.Objects {
		return nil, withFailureCode(errors.New("cached live-object budget exhausted"), "budget")
	}
	g.liveBytes += size
	g.liveObjects++
	return &cachedObject{size: size, role: role}, nil
}

func (g *CachedGoJudge) adopt(object *cachedObject, id string) error {
	if !artifactID.MatchString(id) || g.seen[id] {
		return withFailureCode(errors.New("cached identity unavailable or reused"), "ownership")
	}
	object.id = id
	g.objects[id] = object
	g.seen[id] = true
	return nil
}

func (g *CachedGoJudge) release(object *cachedObject) {
	if object == nil || object.released {
		return
	}
	object.released = true
	g.liveBytes -= object.size
	g.liveObjects--
	if object.id != "" {
		delete(g.objects, object.id)
	}
}

func (g *CachedGoJudge) upload(ctx context.Context, data []byte, role string) (*cachedObject, error) {
	object, err := g.reserve(int64(len(data)), role)
	if err != nil {
		return nil, tagFailure(err, "upload", role, "")
	}
	if err = g.step(int64(len(data)), false); err != nil {
		g.release(object)
		return nil, tagFailure(err, "upload", role, "")
	}
	id, err := g.files.UploadFile(ctx, data)
	if err != nil {
		return object, tagFailure(cachedError(ctx, "cached upload receipt unavailable", err), "upload", role, "")
	}
	if err = g.adopt(object, id); err != nil {
		return object, tagFailure(err, "upload", role, "")
	}
	raw, err := g.read(ctx, object, int64(len(data)))
	if err != nil {
		return object, err
	}
	if len(raw) != len(data) || sha256.Sum256(raw) != sha256.Sum256(data) {
		return object, failureError("readback", role, "integrity", "cached upload readback integrity failed")
	}
	return object, nil
}

func (g *CachedGoJudge) read(ctx context.Context, object *cachedObject, maximum int64) ([]byte, error) {
	role, phase := "unknown", "readback"
	if object != nil {
		role = object.role
		if role == "stdout" {
			phase = "stdout_read"
		}
	}
	if object == nil || g.objects[object.id] != object || maximum < 0 || maximum > object.size {
		return nil, failureError(phase, role, "ownership", "cached read is not owned")
	}
	if err := g.step(maximum, false); err != nil {
		return nil, tagFailure(err, phase, role, "")
	}
	raw, err := g.files.ReadFile(ctx, object.id, maximum)
	if err != nil {
		return nil, tagFailure(cachedError(ctx, "cached read failed", err), phase, role, "")
	}
	if int64(len(raw)) > maximum {
		return nil, failureError(phase, role, "invalid_evidence", "cached read exceeds budget")
	}
	return raw, nil
}

func (g *CachedGoJudge) remove(ctx context.Context, object *cachedObject) error {
	role := "unknown"
	if object != nil {
		role = object.role
	}
	if object == nil || object.id == "" || g.objects[object.id] != object {
		return failureError("delete", role, "ownership", "cached cleanup is not owned")
	}
	if err := g.step(0, true); err != nil {
		return tagFailure(err, "delete", role, "")
	}
	if err := g.control.Remove(ctx, object.id); err != nil {
		return tagFailure(cachedError(ctx, "cached cleanup failed", err), "delete", role, "")
	}
	g.release(object)
	return nil
}

func (g *CachedGoJudge) cleanup(result *error, objects ...*cachedObject) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, object := range objects {
		if object != nil && object.id != "" && !object.released {
			*result = errors.Join(*result, g.remove(ctx, object))
		}
	}
}

func cachedError(ctx context.Context, label string, err error) error {
	cause := ctx.Err()
	if cause == nil && errors.Is(err, context.DeadlineExceeded) {
		cause = context.DeadlineExceeded
	}
	if cause == nil && errors.Is(err, context.Canceled) {
		cause = context.Canceled
	}
	return errors.Join(withFailureCode(errors.New(label), errorFailureCode(err)), cause)
}
