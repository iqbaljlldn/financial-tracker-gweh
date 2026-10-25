-- name: ListActiveTemplatesScaffold :many
SELECT * FROM understanding.sender_templates
WHERE active = true;
