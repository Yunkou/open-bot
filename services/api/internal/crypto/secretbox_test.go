package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	ct, err := Encrypt(key, []byte("hello-secret"))
	if err != nil {
		t.Fatal(err)
	}
	pt, err := Decrypt(key, ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pt, []byte("hello-secret")) {
		t.Fatalf("got %q", pt)
	}
	// hashed key from passphrase
	k2 := shaKey("dev-passphrase-not-for-prod")
	ct2, _ := Encrypt(k2, []byte("x"))
	pt2, err := Decrypt(k2, ct2)
	if err != nil || string(pt2) != "x" {
		t.Fatal(err, string(pt2))
	}
}

func shaKey(s string) []byte {
	k, _ := Encrypt([]byte(s), []byte("t")) // just to use package; real hash via LoadKey path
	_ = k
	sum := make([]byte, 32)
	copy(sum, []byte(s))
	// use Encrypt with short key which hashes internally
	ct, err := Encrypt([]byte(s), []byte("roundtrip"))
	if err != nil {
		panic(err)
	}
	pt, err := Decrypt([]byte(s), ct)
	if err != nil || string(pt) != "roundtrip" {
		panic("hash path failed")
	}
	return []byte(s)
}
