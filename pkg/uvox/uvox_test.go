package uvox

import (
	"testing"
)

func TestXTEAHex(t *testing.T) {
	// Vector 1: user "admin", key "1234567890123456"
	res1 := XTEAHex([]byte("admin"), []byte("1234567890123456"))
	expected1 := "30564744ec4547a8"
	if res1 != expected1 {
		t.Fatalf("expected %s, got %s", expected1, res1)
	}

	// Vector 2: password "mypassword123", key "mysecretkey12345"
	res2 := XTEAHex([]byte("mypassword123"), []byte("mysecretkey12345"))
	expected2 := "bf253484939d5bbfbede43cf26d77250"
	if res2 != expected2 {
		t.Fatalf("expected %s, got %s", expected2, res2)
	}
}
