package payment

import "errors"

var (
	ErrNotConfigured                 = errors.New("apple iap: not configured")
	ErrInvalidConfig                 = errors.New("apple iap: invalid configuration")
	ErrDuplicateProduct              = errors.New("apple iap: duplicate (product_id, environment) in catalog")
	ErrUnknownProduct                = errors.New("apple iap: product not in catalog")
	ErrEnvironmentNotEntitled        = errors.New("apple iap: transaction environment not allowed for entitlement")
	ErrSandboxFallbackDisabled       = errors.New("apple iap: sandbox fallback disabled")
	ErrEmptyAppAccountToken          = errors.New("apple iap: appAccountToken missing")
	ErrAppAccountTokenMismatch       = errors.New("apple iap: appAccountToken does not match authenticated user")
	ErrSubscriptionOwnershipConflict = errors.New("apple iap: original_transaction_id belongs to another user")
	ErrTransactionRevoked            = errors.New("apple iap: transaction revoked")
	ErrUnsupportedProductType        = errors.New("apple iap: only auto-renewable subscriptions supported")
)
