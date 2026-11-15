package interpolate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTraceint(t *testing.T) {
	got, err := Do(`{{traceint .id}}`, map[string]string{"id": "0af7651916cd43dd8448eb211c80319c"})
	assert.NoError(t, err)
	assert.Equal(t, "12057669342925", got, "0x0af7651916cd")
	got, err = Do(`{{traceint .id}}`, map[string]string{"id": "FFFFFFFFFFFF43DD8448EB211C80319C"})
	assert.NoError(t, err)
	assert.Equal(t, "281474976710655", got, "the largest value, 2^48-1")
	got, err = Do(`{{hashids .salt 8 (traceint .id)}}`, map[string]string{"salt": "billing-salt-1", "id": "0af7651916cd43dd8448eb211c80319c"})
	assert.NoError(t, err)
	assert.NotEmpty(t, got, "traceint is the n of hashids")
}

func TestTraceintRejects(t *testing.T) {
	for _, bad := range []string{"", "0af7651916cd", "zzf7651916cd43dd8448eb211c80319c", "0af7651916cd43dd8448eb211c80319c00"} {
		_, err := traceint(bad)
		assert.Error(t, err, "%q", bad)
	}
}
