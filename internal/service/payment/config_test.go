package payment

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dundunHa/go-serverhttp-template/internal/config"
)

const validProductsJSON = `[{"plan_id":"pro_monthly","product_id":"com.app.pro.monthly","level":1,"environment":"Production","subscription_group_id":"21456789"}]`

func validProdConfig() config.AppleIAPConfig {
	return config.AppleIAPConfig{
		BundleID:                "com.app.example",
		IssuerID:                "issuer-uuid",
		KeyID:                   "ABC123",
		PrivateKey:              "-----BEGIN PRIVATE KEY-----\nMIIBVQIBAD...\n-----END PRIVATE KEY-----\n",
		Products:                validProductsJSON,
		EntitlementEnvironments: "Production",
		EnableSandboxFallback:   false,
		WebhookMaxBodyBytes:     65536,
		StoreRawPayloads:        false,
		AppleAPITimeout:         10 * time.Second,
	}
}

func TestNewCatalog_FailingMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		appEnv   string
		mutate   func(*config.AppleIAPConfig)
		wantErr  error
		wantSubs string
	}{
		{
			name: "missing bundle id returns ErrNotConfigured",
			mutate: func(c *config.AppleIAPConfig) {
				c.BundleID = ""
			},
			wantErr: ErrNotConfigured,
		},
		{
			name: "missing issuer id returns ErrNotConfigured",
			mutate: func(c *config.AppleIAPConfig) {
				c.IssuerID = ""
			},
			wantErr: ErrNotConfigured,
		},
		{
			name: "missing key id returns ErrNotConfigured",
			mutate: func(c *config.AppleIAPConfig) {
				c.KeyID = ""
			},
			wantErr: ErrNotConfigured,
		},
		{
			name: "missing private key and p8 path returns ErrNotConfigured",
			mutate: func(c *config.AppleIAPConfig) {
				c.PrivateKey = ""
				c.P8Path = ""
			},
			wantErr: ErrNotConfigured,
		},
		{
			name: "empty products string returns ErrNotConfigured",
			mutate: func(c *config.AppleIAPConfig) {
				c.Products = ""
			},
			wantErr: ErrNotConfigured,
		},
		{
			name: "empty products array returns ErrNotConfigured",
			mutate: func(c *config.AppleIAPConfig) {
				c.Products = "[]"
			},
			wantErr: ErrNotConfigured,
		},
		{
			name: "empty entitlement environments returns ErrNotConfigured",
			mutate: func(c *config.AppleIAPConfig) {
				c.EntitlementEnvironments = ""
			},
			wantErr: ErrNotConfigured,
		},
		{
			name: "whitespace-only entitlement environments returns ErrNotConfigured",
			mutate: func(c *config.AppleIAPConfig) {
				c.EntitlementEnvironments = "   ,  ,"
			},
			wantErr: ErrNotConfigured,
		},
		{
			name: "duplicate (product_id, environment) returns ErrInvalidConfig",
			mutate: func(c *config.AppleIAPConfig) {
				c.Products = `[
					{"plan_id":"pro_monthly","product_id":"com.app.pro.monthly","level":1,"environment":"Production"},
					{"plan_id":"pro_monthly_dup","product_id":"com.app.pro.monthly","level":1,"environment":"Production"}
				]`
			},
			wantErr: ErrDuplicateProduct,
		},
		{
			name: "duplicate also reports as ErrInvalidConfig via Is chain",
			mutate: func(c *config.AppleIAPConfig) {
				c.Products = `[
					{"plan_id":"pro_monthly","product_id":"com.app.pro.monthly","level":1,"environment":"Production"},
					{"plan_id":"pro_monthly","product_id":"com.app.pro.monthly","level":1,"environment":"Production"}
				]`
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "malformed products json returns ErrInvalidConfig",
			mutate: func(c *config.AppleIAPConfig) {
				c.Products = `[{"plan_id":"pro_monthly", oops}]`
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "product missing plan_id returns ErrInvalidConfig",
			mutate: func(c *config.AppleIAPConfig) {
				c.Products = `[{"product_id":"com.app.pro.monthly","level":1,"environment":"Production"}]`
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "product unknown environment returns ErrInvalidConfig",
			mutate: func(c *config.AppleIAPConfig) {
				c.Products = `[{"plan_id":"pro","product_id":"com.app.pro","level":1,"environment":"Lunar"}]`
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "unknown entitlement environment returns ErrInvalidConfig",
			mutate: func(c *config.AppleIAPConfig) {
				c.EntitlementEnvironments = "Production,Lunar"
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name:   "production with StoreRawPayloads=true returns ErrInvalidConfig",
			appEnv: AppEnvProd,
			mutate: func(c *config.AppleIAPConfig) {
				c.StoreRawPayloads = true
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name:   "production with sandbox-only entitlement env returns ErrInvalidConfig",
			appEnv: AppEnvProd,
			mutate: func(c *config.AppleIAPConfig) {
				c.EntitlementEnvironments = "Sandbox"
				c.Products = `[{"plan_id":"pro_monthly","product_id":"com.app.pro.monthly","level":1,"environment":"Sandbox"}]`
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name:   "production with P8Path and no PrivateKey returns ErrInvalidConfig",
			appEnv: AppEnvProd,
			mutate: func(c *config.AppleIAPConfig) {
				c.PrivateKey = ""
				c.P8Path = "/tmp/AuthKey_ABC123.p8"
			},
			wantErr: ErrInvalidConfig,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := validProdConfig()
			tc.mutate(&cfg)
			appEnv := tc.appEnv
			if appEnv == "" {
				appEnv = "dev"
			}

			cat, err := NewCatalog(cfg, appEnv)
			if cat != nil {
				t.Fatalf("expected nil catalog on error, got %#v", cat)
			}
			if err == nil {
				t.Fatalf("expected error %v, got nil", tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected errors.Is(err, %v) = true, got err=%v", tc.wantErr, err)
			}
		})
	}
}

func TestNewCatalog_ValidProdConfig(t *testing.T) {
	t.Parallel()

	cat, err := NewCatalog(validProdConfig(), AppEnvProd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cat == nil {
		t.Fatal("expected non-nil catalog")
	}
	if cat.BundleID() != "com.app.example" {
		t.Errorf("BundleID = %q", cat.BundleID())
	}
	if cat.IssuerID() != "issuer-uuid" {
		t.Errorf("IssuerID = %q", cat.IssuerID())
	}
	if cat.KeyID() != "ABC123" {
		t.Errorf("KeyID = %q", cat.KeyID())
	}
	if cat.PrivateKeyPEM() == "" {
		t.Error("PrivateKeyPEM empty")
	}
	if cat.AppleAPITimeout() != 10*time.Second {
		t.Errorf("AppleAPITimeout = %v", cat.AppleAPITimeout())
	}
	if cat.WebhookMaxBodyBytes() != 65536 {
		t.Errorf("WebhookMaxBodyBytes = %d", cat.WebhookMaxBodyBytes())
	}
	if cat.EnableSandboxFallback() {
		t.Error("EnableSandboxFallback should default false")
	}
	if cat.StoreRawPayloads() {
		t.Error("StoreRawPayloads should default false")
	}
	if got := cat.Products(); len(got) != 1 || got[0].PlanID != "pro_monthly" {
		t.Errorf("Products = %#v", got)
	}
	if envs := cat.AllowedEntitlementEnvironments(); len(envs) != 1 || envs[0] != EnvProduction {
		t.Errorf("AllowedEntitlementEnvironments = %#v", envs)
	}
}

func TestCatalog_Lookup(t *testing.T) {
	t.Parallel()

	cat, err := NewCatalog(validProdConfig(), AppEnvProd)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	got, err := cat.Lookup("com.app.pro.monthly", EnvProduction)
	if err != nil {
		t.Fatalf("expected hit, got err=%v", err)
	}
	if got.PlanID != "pro_monthly" || got.Level != 1 || got.SubscriptionGroupID != "21456789" {
		t.Errorf("Lookup product = %#v", got)
	}

	if _, err := cat.Lookup("com.app.pro.monthly", EnvSandbox); !errors.Is(err, ErrUnknownProduct) {
		t.Errorf("Lookup wrong env: err=%v", err)
	}
	if _, err := cat.Lookup("com.app.unknown", EnvProduction); !errors.Is(err, ErrUnknownProduct) {
		t.Errorf("Lookup unknown id: err=%v", err)
	}
}

func TestCatalog_IsEntitlementEnvironment(t *testing.T) {
	t.Parallel()

	cfg := validProdConfig()
	cfg.EntitlementEnvironments = "Production,Sandbox"
	cfg.Products = `[
		{"plan_id":"pro_monthly","product_id":"com.app.pro.monthly","level":1,"environment":"Production"},
		{"plan_id":"pro_monthly","product_id":"com.app.pro.monthly","level":1,"environment":"Sandbox"}
	]`

	cat, err := NewCatalog(cfg, "dev")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if !cat.IsEntitlementEnvironment(EnvProduction) {
		t.Error("Production should be entitled")
	}
	if !cat.IsEntitlementEnvironment(EnvSandbox) {
		t.Error("Sandbox should be entitled with explicit list")
	}
	if cat.IsEntitlementEnvironment(Environment("Lunar")) {
		t.Error("unknown env must not be entitled")
	}

	got := cat.AllowedEntitlementEnvironments()
	if len(got) != 2 || got[0] != EnvProduction || got[1] != EnvSandbox {
		t.Errorf("AllowedEntitlementEnvironments = %#v", got)
	}
}

func TestCatalog_ProductionOnlyRejectsSandboxLookup(t *testing.T) {
	t.Parallel()

	cat, err := NewCatalog(validProdConfig(), AppEnvProd)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if cat.IsEntitlementEnvironment(EnvSandbox) {
		t.Error("Production-only catalog must not entitle Sandbox transactions")
	}
	if cat.EnableSandboxFallback() {
		t.Error("EnableSandboxFallback default should plumb through as false")
	}
}

func TestCatalog_SandboxFallbackFlagPlumbsThrough(t *testing.T) {
	t.Parallel()

	cfg := validProdConfig()
	cfg.EnableSandboxFallback = true

	cat, err := NewCatalog(cfg, "dev")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !cat.EnableSandboxFallback() {
		t.Error("EnableSandboxFallback=true should plumb through")
	}
}

func TestCatalog_ProductsReturnsCopy(t *testing.T) {
	t.Parallel()

	cat, err := NewCatalog(validProdConfig(), AppEnvProd)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := cat.Products()
	got[0].PlanID = "mutated"

	again := cat.Products()
	if again[0].PlanID != "pro_monthly" {
		t.Errorf("Products() must return a defensive copy; got %#v", again)
	}
}

func TestCatalog_AllowedEntitlementEnvironmentsReturnsCopy(t *testing.T) {
	t.Parallel()

	cat, err := NewCatalog(validProdConfig(), AppEnvProd)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := cat.AllowedEntitlementEnvironments()
	got[0] = Environment("Lunar")

	again := cat.AllowedEntitlementEnvironments()
	if again[0] != EnvProduction {
		t.Errorf("AllowedEntitlementEnvironments() must return a defensive copy; got %#v", again)
	}
}

func TestNewCatalog_DevP8PathReadsFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "AuthKey.p8")
	const pem = "-----BEGIN PRIVATE KEY-----\nDEVKEY\n-----END PRIVATE KEY-----\n"
	if err := os.WriteFile(keyPath, []byte(pem), 0o600); err != nil {
		t.Fatalf("seed key file: %v", err)
	}

	cfg := validProdConfig()
	cfg.PrivateKey = ""
	cfg.P8Path = keyPath

	cat, err := NewCatalog(cfg, "dev")
	if err != nil {
		t.Fatalf("dev with P8Path should succeed: %v", err)
	}
	if cat.PrivateKeyPEM() != pem {
		t.Errorf("PrivateKeyPEM mismatch from p8 file")
	}
}

func TestNewCatalog_DevP8PathMissingFileReportsInvalidConfig(t *testing.T) {
	t.Parallel()

	cfg := validProdConfig()
	cfg.PrivateKey = ""
	cfg.P8Path = filepath.Join(t.TempDir(), "missing.p8")

	_, err := NewCatalog(cfg, "dev")
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestNewCatalog_FullyEmptyReturnsNotConfigured(t *testing.T) {
	t.Parallel()

	_, err := NewCatalog(config.AppleIAPConfig{}, "dev")
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("expected ErrNotConfigured, got %v", err)
	}
}
