package workinsight

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedisPayloadKeepsMessageBoundariesAndReadsLegacyText(t *testing.T) {
	raw, err := encodePayload([]string{"latest", "shared history"})
	require.NoError(t, err)
	require.Equal(t, []string{"latest", "shared history"}, decodePayload(raw))
	require.Equal(t, []string{"legacy payload"}, decodePayload(" legacy payload "))
}
