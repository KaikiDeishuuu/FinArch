package apiv1

import (
	"context"
	"database/sql"
	"fmt"

	"finarch/internal/domain/model"
)

// validateRestoreTransactionAmounts rejects backup rows that would bypass the
// transaction service's positive, JSON-safe amount range. Backups before v9
// stored a single REAL yuan value; v9 and later store authoritative source and
// base-currency integer cents.
func validateRestoreTransactionAmounts(ctx context.Context, database *sql.DB) error {
	columns, err := restoreTransactionColumns(ctx, database)
	if err != nil {
		return err
	}

	_, hasAmountCents := columns["amount_cents"]
	_, hasBaseAmountCents := columns["base_amount_cents"]
	_, hasAmountYuan := columns["amount_yuan"]

	switch {
	case hasAmountCents && hasBaseAmountCents:
		return validateRestoreCentsAmounts(ctx, database)
	case !hasAmountCents && !hasBaseAmountCents && hasAmountYuan:
		return validateRestoreLegacyYuanAmounts(ctx, database)
	default:
		return fmt.Errorf(
			"transactions schema has no supported complete amount representation: %w",
			model.ErrTransactionAmountOutOfRange,
		)
	}
}

func restoreTransactionColumns(ctx context.Context, database *sql.DB) (map[string]struct{}, error) {
	rows, err := database.QueryContext(ctx, `PRAGMA table_info(transactions)`)
	if err != nil {
		return nil, fmt.Errorf("inspect transactions amount schema: %w", err)
	}
	defer rows.Close()

	columns := make(map[string]struct{})
	for rows.Next() {
		var (
			columnID    int
			name        string
			declared    string
			notNull     int
			defaultExpr any
			primaryKey  int
		)
		if err := rows.Scan(&columnID, &name, &declared, &notNull, &defaultExpr, &primaryKey); err != nil {
			return nil, fmt.Errorf("read transactions amount schema: %w", err)
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read transactions amount schema: %w", err)
	}
	return columns, nil
}

func validateRestoreCentsAmounts(ctx context.Context, database *sql.DB) error {
	var (
		transactionID any
		amountType    string
		baseType      string
	)
	err := database.QueryRowContext(ctx, `
		SELECT id, typeof(amount_cents), typeof(base_amount_cents)
		FROM transactions
		WHERE typeof(amount_cents) != 'integer'
		   OR amount_cents <= 0
		   OR amount_cents > ?
		   OR typeof(base_amount_cents) != 'integer'
		   OR base_amount_cents <= 0
		   OR base_amount_cents > ?
		LIMIT 1
	`, model.MaxTransactionAmountCents, model.MaxTransactionAmountCents).Scan(
		&transactionID, &amountType, &baseType,
	)
	if err == nil {
		return fmt.Errorf(
			"transaction %v has invalid cents storage (%s/%s): %w",
			transactionID,
			amountType,
			baseType,
			model.ErrTransactionAmountOutOfRange,
		)
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("validate transaction cents amounts: %w", err)
	}
	return nil
}

func validateRestoreLegacyYuanAmounts(ctx context.Context, database *sql.DB) error {
	rows, err := database.QueryContext(ctx, `
		SELECT id, amount_yuan, typeof(amount_yuan)
		FROM transactions
	`)
	if err != nil {
		return fmt.Errorf("read legacy transaction amounts: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			transactionID any
			rawAmount     any
			storageType   string
		)
		if err := rows.Scan(&transactionID, &rawAmount, &storageType); err != nil {
			return fmt.Errorf("read legacy transaction amount: %w", err)
		}
		var amountYuan float64
		switch amount := rawAmount.(type) {
		case int64:
			amountYuan = float64(amount)
		case float64:
			amountYuan = amount
		default:
			return fmt.Errorf(
				"legacy transaction %v has non-numeric amount storage %q: %w",
				transactionID,
				storageType,
				model.ErrTransactionAmountOutOfRange,
			)
		}
		if storageType != "integer" && storageType != "real" {
			return fmt.Errorf(
				"legacy transaction %v has non-numeric amount storage %q: %w",
				transactionID,
				storageType,
				model.ErrTransactionAmountOutOfRange,
			)
		}
		amountCents, conversionErr := model.Money(amountYuan).Cents()
		if conversionErr != nil || model.ValidateTransactionAmountCents(amountCents) != nil {
			return fmt.Errorf(
				"legacy transaction %v amount is outside the supported range: %w",
				transactionID,
				model.ErrTransactionAmountOutOfRange,
			)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read legacy transaction amounts: %w", err)
	}
	return nil
}
