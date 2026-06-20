package gateway

import (
	"testing"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/connectors"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
	"github.com/stretchr/testify/require"
)

func TestProviderAccountMetadataSpecialProviders(t *testing.T) {
	cf, ok := connectors.SpecByID("cloudflare-ai")
	require.True(t, ok)
	_, err := providerAccountMetadata(cf, providerMetadataInput{})
	require.Error(t, err)

	meta, err := providerAccountMetadata(cf, providerMetadataInput{AccountID: "acct-123"})
	require.NoError(t, err)
	require.Equal(t, "acct-123", meta["accountId"])

	azure, ok := connectors.SpecByID("azure")
	require.True(t, ok)
	meta, err = providerAccountMetadata(azure, providerMetadataInput{
		AzureEndpoint:   "https://example.openai.azure.com/",
		AzureDeployment: "prod-gpt",
		AzureAPIVersion: "2024-10-01-preview",
	})
	require.NoError(t, err)
	require.Equal(t, "https://example.openai.azure.com", meta["azure_endpoint"])
	require.Equal(t, "prod-gpt", meta["deployment"])
	require.Equal(t, "2024-10-01-preview", meta["api_version"])

	custom, ok := connectors.SpecByID("custom-openai")
	require.True(t, ok)
	_, err = providerAccountMetadata(custom, providerMetadataInput{})
	require.Error(t, err)
	meta, err = providerAccountMetadata(custom, providerMetadataInput{BaseURL: "https://llm.example.com/v1"})
	require.NoError(t, err)
	require.Equal(t, "https://llm.example.com/v1", meta["base_url"])
}

func TestAccountAuthKindNoAuthProvider(t *testing.T) {
	spec, ok := connectors.SpecByID("searxng")
	require.True(t, ok)
	require.Equal(t, store.AuthNone, accountAuthKind(spec, ""))
	require.Equal(t, store.AuthAPIKey, accountAuthKind(spec, "optional-key"))
}
