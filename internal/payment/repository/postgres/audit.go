package postgres

import (
	"context"
	"database/sql"

	"github.com/LucasLCabral/payment-service/pkg/payment"
)

type AuditRepository struct {
	DB *sql.DB
}

func (r *AuditRepository) Insert(ctx context.Context, tx *sql.Tx, e *payment.AuditEntry) error {
	const q = `
		INSERT INTO audit_log (entity_type, entity_id, action, old_status, new_status, trace_id, actor)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := tx.ExecContext(ctx, q,
		e.EntityType, e.EntityID, e.Action,
		string(e.OldStatus), string(e.NewStatus), e.TraceID, e.Actor,
	)
	return err
}
