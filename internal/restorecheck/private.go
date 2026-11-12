package restorecheck

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/materialstore"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
)

func verifyDrafts(ctx context.Context, tx pgx.Tx, objects *privateVerifier, summary *MaterialSummary) error {
	var after string
	for {
		var d problem.Draft
		err := tx.QueryRow(ctx, `SELECT id,owner_subject_id,source_id,content_version,request_sha256,state,validation_state,document
			FROM problem_drafts WHERE id>$1 ORDER BY id LIMIT 1`, after).Scan(
			&d.ID, &d.Owner, &d.SourceID, &d.Version, &d.ImportRevision, &d.State, &d.ValidationState, &d.Document)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.New("restored draft inventory unavailable")
		}
		d.References, err = draftObjects(ctx, tx, d.ID)
		if err != nil {
			return errors.New("restored draft material inventory invalid")
		}
		if _, err := d.MaterialInventoryIdentity(); err != nil {
			return errors.New("restored draft material inventory invalid")
		}
		for _, ref := range d.References {
			if err := objects.check(ctx, ref.Object, summary); err != nil {
				return err
			}
		}
		summary.Drafts++
		after = d.ID
	}
}

func draftObjects(ctx context.Context, tx pgx.Tx, id string) ([]problem.Reference, error) {
	rows, err := tx.Query(ctx, `SELECT logical_name,sha256,byte_size FROM problem_draft_materials
		WHERE draft_id=$1 ORDER BY logical_name LIMIT 8193`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (problem.Reference, error) {
		var ref problem.Reference
		err := row.Scan(&ref.Name, &ref.Object.SHA256, &ref.Object.Size)
		return ref, err
	})
}

func verifyPlans(ctx context.Context, tx pgx.Tx, objects *privateVerifier, summary *MaterialSummary) error {
	var after, lastDraft string
	var inventory map[materialstore.Object]struct{}
	var identity problem.ImportIdentity
	for {
		plan, err := nextPlan(ctx, tx, after)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if lastDraft != plan.draftID {
			identity, inventory, err = planDraftInventory(ctx, tx, plan.draftID)
			if err != nil {
				return err
			}
			lastDraft = plan.draftID
		}
		if identity != plan.identity {
			return errors.New("restored reference plan does not match its draft")
		}
		for _, ref := range plan.refs {
			if _, ok := inventory[ref]; !ok {
				return errors.New("restored reference plan object is outside its draft inventory")
			}
			if err := objects.check(ctx, ref, summary); err != nil {
				return err
			}
		}
		summary.ReferencePlans++
		if plan.differential {
			summary.DifferentialPlans++
		}
		after = plan.id
	}
}

func planDraftInventory(ctx context.Context, tx pgx.Tx, id string) (problem.ImportIdentity, map[materialstore.Object]struct{}, error) {
	var d problem.Draft
	err := tx.QueryRow(ctx, `SELECT id,owner_subject_id,source_id,content_version,request_sha256,state,validation_state,document
		FROM problem_drafts WHERE id=$1`, id).Scan(&d.ID, &d.Owner, &d.SourceID, &d.Version, &d.ImportRevision, &d.State, &d.ValidationState, &d.Document)
	if err != nil {
		return problem.ImportIdentity{}, nil, errors.New("restored reference plan does not match its draft")
	}
	d.References, err = draftObjects(ctx, tx, d.ID)
	if err != nil {
		return problem.ImportIdentity{}, nil, errors.New("restored draft material inventory invalid")
	}
	identity, err := d.MaterialInventoryIdentity()
	if err != nil {
		return problem.ImportIdentity{}, nil, errors.New("restored draft material inventory invalid")
	}
	refs := make(map[materialstore.Object]struct{}, len(d.References))
	for _, ref := range d.References {
		refs[ref.Object] = struct{}{}
	}
	return identity, refs, nil
}

type checkedPlan struct {
	id, draftID  string
	identity     problem.ImportIdentity
	refs         []materialstore.Object
	differential bool
}

func nextPlan(ctx context.Context, tx pgx.Tx, after string) (checkedPlan, error) {
	row := tx.QueryRow(ctx, `SELECT p.request_id,r.draft_id,r.source_id,r.content_version,r.import_revision,r.source_lock_sha256,r.profile_sha256,
		p.plan_encoding,p.plan_sha256,p.differential_encoding,p.differential_sha256,
		COALESCE(d.owner_subject_id=r.owner_subject_id,false)
		FROM problem_reference_expected_plans p LEFT JOIN problem_reference_requests r ON r.id=p.request_id
		LEFT JOIN problem_drafts d ON d.id=r.draft_id WHERE p.request_id>$1 ORDER BY p.request_id LIMIT 1`, after)
	var r problemreference.Request
	var p problemreference.ExpectedPlan
	var d problemreference.DifferentialExpectedPlan
	var id string
	var differentialDigest pgtype.Text
	var bound bool
	i := &r.Specification.Identity
	err := row.Scan(&id, &r.DraftID, &i.SourceID, &i.Version, &i.ImportRevision, &i.SourceLockSHA256, &r.Specification.ProfileSHA256,
		&p.Encoding, &p.SHA256, &d.Encoding, &differentialDigest, &bound)
	if errors.Is(err, pgx.ErrNoRows) {
		return checkedPlan{}, err
	}
	if err != nil || !bound {
		return checkedPlan{}, errors.New("restored reference plan does not match its draft")
	}
	if differentialDigest.Valid != (d.Encoding != nil) {
		return checkedPlan{}, errors.New("restored differential plan is incomplete")
	}
	if differentialDigest.Valid {
		d.SHA256 = differentialDigest.String
		p.Differential = &d
	}
	if !p.ValidForRequest(r) {
		return checkedPlan{}, errors.New("restored reference plan invalid")
	}
	var formal problem.ReferencePlan
	if err := json.Unmarshal(p.Encoding, &formal); err != nil {
		return checkedPlan{}, errors.New("restored reference plan invalid")
	}
	refs := formal.ReferencedObjects()
	if p.Differential != nil {
		var differential problemvalidation.ConvolutionPlan
		if err := json.Unmarshal(d.Encoding, &differential); err != nil {
			return checkedPlan{}, errors.New("restored differential plan invalid")
		}
		for _, program := range []problemvalidation.ConvolutionProgram{differential.Oracle, differential.Candidate} {
			refs = append(refs, materialstore.Object{SHA256: program.SHA256, Size: program.Size})
		}
	}
	return checkedPlan{id: id, draftID: r.DraftID, identity: *i, refs: refs, differential: p.Differential != nil}, nil
}
