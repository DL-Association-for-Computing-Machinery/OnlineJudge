-- name: DraftByKeyForShare :one
SELECT * FROM problem_drafts WHERE owner_subject_id = @owner AND request_key = @request_key FOR SHARE;

-- name: DraftByOwner :one
SELECT * FROM problem_drafts WHERE id = @id AND owner_subject_id = @owner FOR SHARE;

-- name: CreateDraft :one
INSERT INTO problem_drafts(id,owner_subject_id,request_key,request_sha256,source_id,content_version,document)
VALUES(@id,@owner,@request_key,@request_sha256,@source_id,@content_version,@document)
ON CONFLICT(owner_subject_id,request_key) DO NOTHING
RETURNING *;

-- name: AddDraftMaterial :exec
INSERT INTO problem_draft_materials(draft_id,logical_name,sha256,byte_size)
VALUES(@draft_id,@logical_name,@sha256,@byte_size);

-- name: DraftMaterials :many
SELECT logical_name,sha256,byte_size FROM problem_draft_materials WHERE draft_id = @draft_id ORDER BY logical_name;
