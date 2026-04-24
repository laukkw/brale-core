package pgstore

import (
	"context"
	"fmt"

	"brale-core/internal/store"

	"github.com/jackc/pgx/v5"
)

// ─── Prompt Registry ─────────────────────────────────────────────

func (s *PGStore) SavePromptEntry(ctx context.Context, rec *store.PromptRegistryEntry) error {
	if rec == nil {
		return fmt.Errorf("record is nil")
	}
	const q = `INSERT INTO prompt_registry (role, stage, locale, version, system_prompt, description, active)
	VALUES ($1,$2,$3,$4,$5,$6,$7)
	ON CONFLICT (role, stage, locale, version) DO UPDATE SET
		system_prompt = EXCLUDED.system_prompt,
		description = EXCLUDED.description,
		active = EXCLUDED.active,
		updated_at = now()
	RETURNING id, created_at, updated_at`
	return s.queryRow(ctx, q,
		rec.Role, rec.Stage, rec.Locale, rec.Version, rec.SystemPrompt, rec.Description, rec.Active,
	).Scan(&rec.ID, &rec.CreatedAt, &rec.UpdatedAt)
}

func (s *PGStore) FindActivePrompt(ctx context.Context, role, stage, locale string) (store.PromptRegistryEntry, bool, error) {
	row := s.queryRow(ctx,
		`SELECT id, role, stage, locale, version, system_prompt, description, active, created_at, updated_at
		 FROM prompt_registry
		 WHERE role = $1 AND stage = $2 AND locale = $3 AND active = true
		 ORDER BY created_at DESC LIMIT 1`, role, stage, locale)
	rec, err := scanPromptRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return store.PromptRegistryEntry{}, false, nil
		}
		return store.PromptRegistryEntry{}, false, err
	}
	return rec, true, nil
}

func (s *PGStore) ListPromptEntries(ctx context.Context, role string, activeOnly bool) ([]store.PromptRegistryEntry, error) {
	q := `SELECT id, role, stage, locale, version, system_prompt, description, active, created_at, updated_at
	      FROM prompt_registry WHERE 1=1`
	args := []any{}
	idx := 1
	if role != "" {
		q += fmt.Sprintf(" AND role = $%d", idx)
		args = append(args, role)
		idx++
	}
	if activeOnly {
		q += fmt.Sprintf(" AND active = $%d", idx)
		args = append(args, true)
		idx++
	}
	q += " ORDER BY role, stage, locale, created_at DESC"
	rows, err := s.query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []store.PromptRegistryEntry
	for rows.Next() {
		rec, err := scanPromptRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func scanPromptRow(row scannable) (store.PromptRegistryEntry, error) {
	var r store.PromptRegistryEntry
	if err := row.Scan(
		&r.ID, &r.Role, &r.Stage, &r.Locale, &r.Version,
		&r.SystemPrompt, &r.Description, &r.Active,
		&r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return store.PromptRegistryEntry{}, err
	}
	return r, nil
}
