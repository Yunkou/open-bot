package db

// avatarV2UserSetMarker gates the one-time "mark existing avatars as user_set" step.
const avatarV2UserSetMarker = "avatar_v2_user_set"

// migrateAvatarV2 runs at startup (after the avatar_user_set column + legacy shape map in schema).
//
//  1. ONE TIME (guarded by app_migrations): rows that already had shape+color are marked
//     avatar_user_set=true so they are kept as-is, even if their pair collides.
//     Later auto-assigned rows (create/clone/admin → avatar_user_set=false) are never flipped.
//  2. Every startup (idempotent): legacy shape ids are canonicalized, and only empty/invalid
//     avatars are backfilled via assignAvatarUnderLock (same per-user advisory lock as
//     create/clone/admin), avoiding pairs already taken. Backfilled rows stay user_set=false.
//
// List/Get never write avatars; this is the only backfill path.
func (d *DB) migrateAvatarV2() error {
	if _, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS app_migrations (
  name       TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`); err != nil {
		return err
	}
	if err := d.markExistingAvatarsUserSetOnce(); err != nil {
		return err
	}

	rows, err := d.SQL.Query(`
SELECT id, COALESCE(user_id,''), COALESCE(avatar_shape,''), COALESCE(avatar_color,'')
FROM agents
WHERE deleted_at IS NULL
ORDER BY user_id, created_at ASC, id ASC
`)
	if err != nil {
		return err
	}
	type row struct{ id, userID, shape, color string }
	var pending []row
	var legacy []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.userID, &r.shape, &r.color); err != nil {
			_ = rows.Close()
			return err
		}
		if canon := CanonicalAvatarShape(r.shape); canon != "" && canon != r.shape {
			r.shape = canon
			legacy = append(legacy, r)
		}
		if !IsAllowedAvatarShape(r.shape) || !IsAllowedAvatarColor(r.color) {
			pending = append(pending, r)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	for _, r := range legacy {
		if _, err := d.SQL.Exec(`UPDATE agents SET avatar_shape=$1 WHERE id=$2`, r.shape, r.id); err != nil {
			return err
		}
	}
	for _, r := range pending {
		if err := d.backfillAvatar(r.userID, r.id, r.shape, r.color); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) markExistingAvatarsUserSetOnce() error {
	tx, err := d.SQL.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(
		`INSERT INTO app_migrations (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`,
		avatarV2UserSetMarker,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // already applied
	}
	if _, err := tx.Exec(`
UPDATE agents
SET avatar_user_set = TRUE
WHERE deleted_at IS NULL
  AND avatar_user_set = FALSE
  AND avatar_shape <> ''
  AND avatar_color <> ''
`); err != nil {
		return err
	}
	return tx.Commit()
}

// backfillAvatar assigns a free pair to one empty/invalid avatar under the shared assign lock.
func (d *DB) backfillAvatar(userID, agentID, shape, color string) error {
	tx, err := d.SQL.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if !IsAllowedAvatarShape(shape) {
		shape = ""
	}
	color = NormalizeAvatarColor(color)
	s, c, err := assignAvatarUnderLock(tx, userID, agentID, shape, color)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE agents SET avatar_shape=$1, avatar_color=$2, avatar_user_set=FALSE
		 WHERE id=$3 AND deleted_at IS NULL`,
		s, c, agentID,
	); err != nil {
		return err
	}
	return tx.Commit()
}
