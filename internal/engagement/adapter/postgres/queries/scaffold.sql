-- name: ListNotificationJobsScaffold :many
SELECT * FROM engagement.notification_jobs
WHERE user_id = $1;
