package messaging

import (
	"errors"
	"testing"
)

func TestPermanent(t *testing.T) {
	inner := errors.New("root")
	w := Permanent(inner)
	if !IsPermanent(w) {
		t.Fatal("IsPermanent should be true")
	}
	if !errors.Is(w, inner) {
		t.Fatal("errors.Is unwrap")
	}
	if IsPermanent(inner) {
		t.Fatal("raw error should not be permanent")
	}
}
