package wait

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ovh/venom"
)

func TestWaitSeconds(t *testing.T) {
	for _, given := range []interface{}{"0.05", 0.05} {
		start := time.Now()
		out, err := Executor{}.Run(context.Background(), venom.TestStep{"seconds": given, "reason": "absence window"})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, time.Since(start), 50*time.Millisecond)
		assert.Equal(t, Result{Seconds: 0.05, Reason: "absence window"}, out.(Result))
	}
}

func TestWaitRejectsBadSeconds(t *testing.T) {
	for _, bad := range []interface{}{"later", "-1", ""} {
		_, err := Executor{}.Run(context.Background(), venom.TestStep{"seconds": bad})
		assert.Error(t, err, "%v", bad)
	}
}

func TestWaitEndsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Executor{}.Run(ctx, venom.TestStep{"seconds": 60})
	assert.ErrorIs(t, err, context.Canceled)
}
