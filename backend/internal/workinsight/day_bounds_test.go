package workinsight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUsageDayBoundsPreservesLocalDayAcrossDST(t *testing.T) {
	date := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	start, end, err := usageDayBounds(date, "America/New_York")

	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC), start)
	require.Equal(t, 23*time.Hour, end.Sub(start))
}
