package crypto

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVaultTransitOperations(t *testing.T) {
	mac := base64.StdEncoding.EncodeToString(make([]byte, 32))
	plaintext := base64.StdEncoding.EncodeToString([]byte("test national id"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Vault-Token"); got != "test-token" {
			t.Errorf("Vault token header = %q, want test-token", got)
		}
		switch r.URL.Path {
		case "/v1/transit/encrypt/nid":
			fmt.Fprint(w, `{"data":{"ciphertext":"vault:v1:encrypted"}}`)
		case "/v1/transit/decrypt/nid":
			fmt.Fprintf(w, `{"data":{"plaintext":%q}}`, plaintext)
		case "/v1/transit/hmac/nid-hmac/sha2-256":
			fmt.Fprintf(w, `{"data":{"hmac":%q}}`, "vault:v1:"+mac)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	transit, err := NewVaultTransit(server.URL, "test-token", "transit")
	if err != nil {
		t.Fatal(err)
	}

	ciphertext, err := transit.Encrypt(t.Context(), "nid", []byte("test national id"))
	if err != nil || string(ciphertext) != "vault:v1:encrypted" {
		t.Fatalf("Encrypt() = %q, %v", ciphertext, err)
	}
	decoded, err := transit.Decrypt(t.Context(), "nid", ciphertext)
	if err != nil || string(decoded) != "test national id" {
		t.Fatalf("Decrypt() = %q, %v", decoded, err)
	}
	index, err := transit.BlindIndex(t.Context(), "nid-hmac", "test national id")
	if err != nil || len(index) != 32 {
		t.Fatalf("BlindIndex() length = %d, error = %v", len(index), err)
	}
	if strings.Contains(string(index), "vault:") {
		t.Fatal("BlindIndex() returned the Vault prefix instead of raw HMAC bytes")
	}
}

func TestVaultTransitRejectsRemoteHTTP(t *testing.T) {
	if _, err := NewVaultTransit("http://vault.example.test", "token", "transit"); err == nil {
		t.Fatal("NewVaultTransit() accepted non-TLS remote address")
	}
}
