package crypto

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Transit provides the cryptographic operations needed by register identifiers.
type Transit interface {
	Encrypt(context.Context, string, []byte) ([]byte, error)
	Decrypt(context.Context, string, []byte) ([]byte, error)
	BlindIndex(context.Context, string, string) ([]byte, error)
}

// VaultTransit calls Vault's Transit encrypt, decrypt, and HMAC endpoints.
type VaultTransit struct {
	baseURL string
	token   string
	mount   string
	client  *http.Client
}

// NewVaultTransit creates a client for a Vault Transit mount.
func NewVaultTransit(address, token, mount string) (*VaultTransit, error) {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, fmt.Errorf("Vault address must be an absolute HTTP(S) URL")
	}
	if parsed.Scheme != "https" {
		host, _, splitErr := net.SplitHostPort(parsed.Host)
		if splitErr != nil {
			host = parsed.Host
		}
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return nil, fmt.Errorf("Vault must use HTTPS except on loopback")
		}
	}
	return &VaultTransit{
		baseURL: strings.TrimRight(address, "/"),
		token:   token,
		mount:   strings.Trim(mount, "/"),
		client:  &http.Client{Timeout: 5 * time.Second},
	}, nil
}

// Encrypt protects plaintext with the configured Transit encryption key.
func (v *VaultTransit) Encrypt(ctx context.Context, key string, plaintext []byte) ([]byte, error) {
	var response transitResponse
	payload := map[string]string{"plaintext": base64.StdEncoding.EncodeToString(plaintext)}
	if err := v.post(ctx, "encrypt/"+url.PathEscape(key), payload, &response); err != nil {
		return nil, err
	}
	if response.Data.Ciphertext == "" {
		return nil, fmt.Errorf("Vault Transit returned no ciphertext")
	}
	return []byte(response.Data.Ciphertext), nil
}

// Decrypt recovers plaintext using the configured Transit encryption key.
func (v *VaultTransit) Decrypt(ctx context.Context, key string, ciphertext []byte) ([]byte, error) {
	var response transitResponse
	payload := map[string]string{"ciphertext": string(ciphertext)}
	if err := v.post(ctx, "decrypt/"+url.PathEscape(key), payload, &response); err != nil {
		return nil, err
	}
	plaintext, err := base64.StdEncoding.DecodeString(response.Data.Plaintext)
	if err != nil {
		return nil, fmt.Errorf("decode Vault Transit plaintext: %w", err)
	}
	return plaintext, nil
}

// BlindIndex returns the raw SHA-256 HMAC bytes from the Transit HMAC endpoint.
func (v *VaultTransit) BlindIndex(ctx context.Context, key, value string) ([]byte, error) {
	var response transitResponse
	payload := map[string]string{"input": base64.StdEncoding.EncodeToString([]byte(value))}
	if err := v.post(ctx, "hmac/"+url.PathEscape(key)+"/sha2-256", payload, &response); err != nil {
		return nil, err
	}
	parts := strings.Split(response.Data.HMAC, ":")
	if len(parts) != 3 || parts[0] != "vault" || parts[1] == "" {
		return nil, fmt.Errorf("Vault Transit returned an invalid HMAC")
	}
	mac, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("decode Vault Transit HMAC: %w", err)
	}
	if len(mac) != 32 {
		return nil, fmt.Errorf("Vault Transit returned an invalid HMAC length")
	}
	return mac, nil
}

func (v *VaultTransit) post(ctx context.Context, operation string, payload any, result *transitResponse) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Vault Transit request: %w", err)
	}
	endpoint := v.baseURL + "/v1/" + v.mount + "/" + operation
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Vault Transit request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Vault-Token", v.token)

	response, err := v.client.Do(request)
	if err != nil {
		return fmt.Errorf("call Vault Transit: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Vault Transit returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("decode Vault Transit response: %w", err)
	}
	return nil
}

type transitResponse struct {
	Data struct {
		Ciphertext string `json:"ciphertext"`
		Plaintext  string `json:"plaintext"`
		HMAC       string `json:"hmac"`
	} `json:"data"`
}
