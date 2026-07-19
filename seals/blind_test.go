package seals

import (
	"encoding/hex"
	"testing"
)

func TestBlindSealStrictEncoding(t *testing.T) {
	txid := make([]byte, 32)
	for i := range txid {
		txid[i] = byte(i)
	}
	seal, err := NewBlindSeal(txid, 0x11223344, 0x0102030405060708)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := seal.StrictBytes()
	if err != nil {
		t.Fatal(err)
	}
	want := hex.EncodeToString(txid) + "44332211" + "0807060504030201"
	if got := hex.EncodeToString(encoded); got != want {
		t.Fatalf("strict encoding %s != %s", got, want)
	}
	first, err := seal.Conceal()
	if err != nil {
		t.Fatal(err)
	}
	second, _ := seal.Conceal()
	if first != second {
		t.Fatal("seal concealment must be deterministic")
	}
	wantConcealed := "utxob:0Or8MLTb-YCdQqTa-dExptV~-z7EXh~0-LAjTBDe-Wek7UD3-DY2Mm"
	if got := first.String(); got != wantConcealed {
		t.Fatalf("Rust differential concealed seal %s != %s", got, wantConcealed)
	}
}
