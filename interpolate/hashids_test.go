package interpolate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashids(t *testing.T) {
	cases := []struct {
		salt string
		n    string
		want string
	}{
		{"billing-salt-1", "1", "NloBboqV"},
		{"billing-salt-1", "99", "ePo8DzoJ"},
		{"billing-salt-1", "12345", "loBZ3moq"},
		{"billing-salt-1", "1731686400000001", "BnqdQgNx2NW"},
		{"Zq9#x", "1", "mKem1e90"},
		{"Zq9#x", "99", "NnDzo2eA"},
		{"Zq9#x", "12345", "KempvKD9"},
		{"Zq9#x", "1731686400000001", "mob64R5z259"},
		{"", "1", ""},
	}
	for _, c := range cases {
		got, err := Do(`{{hashids .salt 8 .n}}`, map[string]string{"salt": c.salt, "n": c.n})
		assert.NoError(t, err)
		assert.Equal(t, c.want, got, "salt=%q n=%s", c.salt, c.n)
	}
}
