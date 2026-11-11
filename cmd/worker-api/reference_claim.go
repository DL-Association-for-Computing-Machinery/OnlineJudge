package main

import (
	"crypto/x509"
	"errors"
	"net/http"

	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

func referenceExecutionEnabled(value, nodeID, root string) (bool, error) {
	if value == "" {
		return false, nil
	}
	if value != "1" || !problemreference.ValidReferenceNodeID(nodeID) || root == "" {
		return false, errors.New("reference execution requires explicit opt-in, a reference node and material root")
	}
	return true, nil
}

func configureReferenceClaims(next http.Handler, pool *pgxpool.Pool, roots *x509.CertPool, nodeID string, objects problem.ReferenceObjectReader, execution bool) (http.Handler, error) {
	if execution && (!problemreference.ValidReferenceNodeID(nodeID) || objects == nil) {
		return nil, problemreference.ErrInvalid
	}
	if nodeID == "" {
		return next, nil
	}
	if !problemreference.ValidReferenceNodeID(nodeID) {
		return nil, problemreference.ErrInvalid
	}
	claims, err := referenceClaimer(pool, nodeID)
	if err != nil {
		return nil, err
	}
	var materials *problemreference.ReferenceMaterials
	if objects != nil {
		materials, err = problemreference.NewReferenceMaterials(claims, objects)
		if err != nil {
			return nil, err
		}
	}
	if execution {
		return workerapi.WithReferenceExecution(next, claims, claims, roots, materials, claims), nil
	}
	return workerapi.WithReferenceClaims(next, claims, roots, materials), nil
}

func referenceClaimer(pool *pgxpool.Pool, nodeID string) (*referencepg.ReferenceClaimer, error) {
	author, err := identitypg.NewProblemAccess(pool)
	if err != nil {
		return nil, err
	}
	access, err := problempg.NewReferenceAccess(author)
	if err != nil {
		return nil, err
	}
	store, err := referencepg.New(pool, access)
	if err != nil {
		return nil, err
	}
	return referencepg.NewReferenceClaimer(store, submissionpg.LockReferenceNode, nodeID, problemvalidation.SourceLockSHA256)
}
