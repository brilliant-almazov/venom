package interpolate

import (
	hashids "github.com/speps/go-hashids/v2"
)

// hashid encodes n by the hashids algorithm on the default alphabet, the given salt and minimum length.
// Stock Venom has no hashids function: a suite that sends an id or an idempotency key hashed on a salt read
// from the API builds it here instead of a script. An empty salt gives an empty string, as the API gave nothing to hash on.
func hashid(salt string, minLength interface{}, n interface{}) (string, error) {
	if salt == "" {
		return "", nil
	}
	data := hashids.NewData()
	data.Salt = salt
	data.MinLength = int(toInt64(minLength))
	h, err := hashids.NewWithData(data)
	if err != nil {
		return "", err
	}
	return h.EncodeInt64([]int64{toInt64(n)})
}
