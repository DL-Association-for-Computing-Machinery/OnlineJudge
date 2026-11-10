package main

import (
	"crypto/x509"
	"net/http"

	identitypg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/identity/postgres"
	problempg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problem/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference"
	referencepg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemreference/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/problemvalidation"
	submissionpg "github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/submission/postgres"
	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/workerapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

func configureReferenceClaims(next http.Handler, pool *pgxpool.Pool, roots *x509.CertPool, nodeID string) (http.Handler, error) {
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
	return workerapi.WithReferenceClaims(next, claims, roots), nil
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
