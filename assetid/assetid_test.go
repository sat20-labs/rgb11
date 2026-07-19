package assetid

import (
	"errors"
	"testing"
)

func TestTickerRoundTrip(t *testing.T) {
	official := "rgb:Ar4ouaLv-b7f7Dc_-z5EMvtu-FA5KNh1-nlae~jk-8xMBo7E"
	ticker, err := Ticker(official)
	if err != nil {
		t.Fatal(err)
	}
	if ticker != "Ar4ouaLv-b7f7Dc_-z5EMvtu-FA5KNh1-nlae~jk-8xMBo7E" {
		t.Fatalf("unexpected ticker %q", ticker)
	}
	got, err := AssetID(ticker)
	if err != nil {
		t.Fatal(err)
	}
	if got != official {
		t.Fatalf("round trip mismatch %q != %q", got, official)
	}
}

func TestTickerRejectsDelimiter(t *testing.T) {
	if _, err := Ticker("rgb:a:b"); !errors.Is(err, ErrInvalidTicker) {
		t.Fatalf("expected ErrInvalidTicker, got %v", err)
	}
}
