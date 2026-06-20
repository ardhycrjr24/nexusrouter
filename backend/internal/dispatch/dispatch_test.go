package dispatch

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/config"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/core"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/crypto"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/vault"
)

type fakeConnectorSource struct {
	conn core.Connector
}

func (s fakeConnectorSource) Get(provider string) (core.Connector, error) {
	return s.conn, nil
}

type fakeConnector struct{}

func (fakeConnector) ID() string            { return "openai" }
func (fakeConnector) Dialect() core.Dialect { return core.DialectOpenAI }
func (fakeConnector) Chat(context.Context, *core.ChatRequest, core.Credentials) (*core.ChatResponse, error) {
	return nil, nil
}
func (fakeConnector) Stream(context.Context, *core.ChatRequest, core.Credentials, core.StreamConfig) (<-chan core.StreamChunk, error) {
	ch := make(chan core.StreamChunk)
	close(ch)
	return ch, nil
}

func newDispatchTest(t *testing.T, accounts ...store.Account) (*Dispatcher, *store.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: ":memory:"}, t.TempDir())
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Tenants().EnsureDefault(ctx))
	t.Cleanup(func() { _ = db.Close() })

	mk, err := crypto.GenerateMasterKey()
	require.NoError(t, err)
	sealer, err := crypto.NewSealer(mk)
	require.NoError(t, err)
	v := vault.New(sealer)

	for i := range accounts {
		require.NoError(t, v.Seal(&accounts[i], vault.NewSecret{APIKey: "sk-test"}))
		require.NoError(t, db.Accounts().Create(ctx, accounts[i]))
	}

	d := New(fakeConnectorSource{conn: fakeConnector{}}, db.Accounts(), v)
	d.SetRoutingSource(db.Routing())
	return d, db
}

func testAccount(id string, priority int) store.Account {
	now := time.Now()
	return store.Account{
		ID:        id,
		TenantID:  store.DefaultTenantID,
		Provider:  "openai",
		Label:     id,
		AuthKind:  store.AuthAPIKey,
		Priority:  priority,
		Metadata:  "{}",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestPlanWith_AccountRoundRobinRotatesProviderAccounts(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t,
		testAccount("acc-1", 10),
		testAccount("acc-2", 20),
		testAccount("acc-3", 30),
	)

	targets := []Target{{Provider: "openai", Model: "gpt-4o"}}
	opts := PlanOptions{AccountStrategy: StrategyRoundRobin}

	var got []string
	for i := 0; i < 4; i++ {
		attempts, err := d.PlanWith(ctx, store.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
		require.NoError(t, err)
		require.NotEmpty(t, attempts)
		got = append(got, attempts[0].Account.ID)
	}

	require.Equal(t, []string{"acc-1", "acc-2", "acc-3", "acc-1"}, got)
}

func TestPlanWith_AccountRoundRobinHonorsStickyLimit(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t,
		testAccount("acc-1", 10),
		testAccount("acc-2", 20),
	)

	targets := []Target{{Provider: "openai", Model: "gpt-4o"}}
	opts := PlanOptions{AccountStrategy: StrategyRoundRobin, AccountStickyLimit: 2}

	var got []string
	for i := 0; i < 5; i++ {
		attempts, err := d.PlanWith(ctx, store.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
		require.NoError(t, err)
		require.NotEmpty(t, attempts)
		got = append(got, attempts[0].Account.ID)
	}

	require.Equal(t, []string{"acc-1", "acc-1", "acc-2", "acc-2", "acc-1"}, got)
}

func TestPlanWith_SmartRoundRobinPinsAffinityKey(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t,
		testAccount("acc-1", 10),
		testAccount("acc-2", 20),
		testAccount("acc-3", 30),
	)

	targets := []Target{{Provider: "openai", Model: "gpt-4o"}}
	opts := PlanOptions{AccountStrategy: StrategySmartRoundRobin, AccountAffinityKey: "thread-a"}

	attempts, err := d.PlanWith(ctx, store.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
	require.NoError(t, err)
	require.Equal(t, "acc-1", attempts[0].Account.ID)

	attempts, err = d.PlanWith(ctx, store.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
	require.NoError(t, err)
	require.Equal(t, "acc-1", attempts[0].Account.ID)

	opts.AccountAffinityKey = "thread-b"
	attempts, err = d.PlanWith(ctx, store.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
	require.NoError(t, err)
	require.Equal(t, "acc-2", attempts[0].Account.ID)

	opts.AccountAffinityKey = "thread-a"
	attempts, err = d.PlanWith(ctx, store.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
	require.NoError(t, err)
	require.Equal(t, "acc-1", attempts[0].Account.ID)
}

func TestPlanWith_ProviderAccountStrategyOverride(t *testing.T) {
	ctx := context.Background()
	d, _ := newDispatchTest(t,
		testAccount("acc-1", 10),
		testAccount("acc-2", 20),
	)

	targets := []Target{{Provider: "openai", Model: "gpt-4o"}}
	opts := PlanOptions{
		AccountStrategy: StrategyFallback,
		ProviderAccountStrategies: map[string]AccountRoutingOptions{
			"openai": {Strategy: StrategyRoundRobin},
		},
	}

	var got []string
	for i := 0; i < 3; i++ {
		attempts, err := d.PlanWith(ctx, store.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
		require.NoError(t, err)
		got = append(got, attempts[0].Account.ID)
	}

	require.Equal(t, []string{"acc-1", "acc-2", "acc-1"}, got)
}

func TestPlanWith_TargetRoundRobinRotatesComboTargets(t *testing.T) {
	ctx := context.Background()
	d, db := newDispatchTest(t, testAccount("acc-1", 10))
	now := time.Now()
	require.NoError(t, db.Chains().Create(ctx, store.Chain{
		ID:        "chain-1",
		TenantID:  store.DefaultTenantID,
		Name:      "combo",
		Strategy:  string(StrategyRoundRobin),
		CreatedAt: now,
		UpdatedAt: now,
	}))

	targets := []Target{
		{Provider: "openai", Model: "gpt-4o"},
		{Provider: "openai", Model: "gpt-5"},
	}
	opts := PlanOptions{Strategy: StrategyRoundRobin, ChainID: "chain-1"}

	var got []string
	for i := 0; i < 3; i++ {
		attempts, err := d.PlanWith(ctx, store.DefaultTenantID, targets, core.NewCapabilitySet(), opts)
		require.NoError(t, err)
		got = append(got, attempts[0].Target.Model)
	}

	require.Equal(t, []string{"gpt-4o", "gpt-5", "gpt-4o"}, got)
}

func TestAdvanceRotationStateHonorsStickyLimit(t *testing.T) {
	cursor, nextCursor, hits := advanceRotationState(3, 0, 0, 2)
	require.Equal(t, 0, cursor)
	require.Equal(t, 0, nextCursor)
	require.Equal(t, 1, hits)

	cursor, nextCursor, hits = advanceRotationState(3, nextCursor, hits, 2)
	require.Equal(t, 0, cursor)
	require.Equal(t, 1, nextCursor)
	require.Equal(t, 0, hits)
}
