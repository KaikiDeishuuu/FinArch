# Balance repair migration (v21)

Migration `v21` recomputes `accounts.balance_cents` from `transactions.base_amount_cents` for every account.

- Purpose: repair historical balance corruption from legacy trigger definitions that summed `amount_cents` (original currency) instead of normalized `base_amount_cents`.
- Behavior: deterministic and idempotent full rebuild.
- Scope: all accounts (personal/public), all modes.
- Safety: rerunning produces the same results because balances are replaced by a pure aggregation query.

After deployment, LIFE overview, Settings account balances, and transaction/statistics totals read the same normalized source.
