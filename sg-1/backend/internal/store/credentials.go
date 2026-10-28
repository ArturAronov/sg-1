package store

import (
	"context"
	"errors"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/domain"
)

func (s *Store) LLMCredentials(ctx context.Context, userID string) ([]domain.LLMCredentialStatus, error) {
	rows, err := s.Pool.Query(ctx, `SELECT provider,updated_at FROM llm_credentials WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.LLMCredentialStatus{}
	for rows.Next() {
		var status domain.LLMCredentialStatus
		if err := rows.Scan(&status.Provider, &status.UpdatedAt); err != nil {
			return nil, err
		}
		status.Configured = true
		result = append(result, status)
	}
	return result, rows.Err()
}

func (s *Store) SaveLLMCredential(ctx context.Context, userID, provider string, ciphertext []byte) (domain.LLMCredentialStatus, error) {
	status := domain.LLMCredentialStatus{Provider: provider, Configured: true}
	if !domain.ValidLLMProvider(provider) || len(ciphertext) < 45 {
		return status, errors.New("invalid encrypted credential")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return status, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `INSERT INTO llm_credentials(user_id,provider,key_ciphertext) VALUES($1,$2,$3)
		ON CONFLICT(user_id,provider) DO UPDATE SET key_ciphertext=EXCLUDED.key_ciphertext,updated_at=now()
		RETURNING updated_at`, userID, provider, ciphertext).Scan(&status.UpdatedAt)
	if err != nil {
		return status, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,metadata) VALUES($1,'llm_credential.saved',jsonb_build_object('provider',$2::text))`, userID, provider); err != nil {
		return status, err
	}
	return status, tx.Commit(ctx)
}

func (s *Store) DeleteLLMCredential(ctx context.Context, userID, provider string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `DELETE FROM llm_credentials WHERE user_id=$1 AND provider=$2`, userID, provider)
	if err != nil {
		return err
	}
	if result.RowsAffected() > 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_id,action,metadata) VALUES($1,'llm_credential.removed',jsonb_build_object('provider',$2::text))`, userID, provider); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
