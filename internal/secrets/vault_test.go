package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestVaultBackend_GetItems(t *testing.T) {
	// Mock Vault KV v2 API responses
	mux := http.NewServeMux()

	// LIST secret/metadata/agents (Vault client sends GET with ?list=true)
	mux.HandleFunc("/v1/secret/metadata/agents", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"keys": []string{"test-agent", "another-agent"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// GET secret/data/agents/test-agent
	mux.HandleFunc("/v1/secret/data/agents/test-agent", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{
					"app_id":          "123456",
					"installation_id": "78901234",
					"agent_token":     "dGVzdC10b2tlbg==",
					"private_key":     "-----BEGIN RSA PRIVATE KEY-----\\ntest\\n-----END RSA PRIVATE KEY-----",
					"identity_name":   "Test Agent",
					"identity_email":  "test@example.com",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// GET secret/data/agents/another-agent
	mux.HandleFunc("/v1/secret/data/agents/another-agent", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{
					"app_id":          "654321",
					"installation_id": "43210987",
					"agent_token":     "b3RoZXItdG9rZW4=",
					"private_key":     "-----BEGIN RSA PRIVATE KEY-----\\nother\\n-----END RSA PRIVATE KEY-----",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// Token lookup (for NewVaultBackend verification)
	mux.HandleFunc("/v1/auth/token/lookup-self", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"id": "test-token",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Set VAULT_TOKEN for the backend constructor
	os.Setenv("VAULT_TOKEN", "test-token")
	defer os.Unsetenv("VAULT_TOKEN")

	backend, err := NewVaultBackend(server.URL, "secret")
	if err != nil {
		t.Fatalf("NewVaultBackend failed: %v", err)
	}

	items, err := backend.GetItems(context.Background(), "agents")
	if err != nil {
		t.Fatalf("GetItems failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Find test-agent
	var testAgent *Item
	for _, item := range items {
		if item.Name == "test-agent" {
			testAgent = item
			break
		}
	}
	if testAgent == nil {
		t.Fatal("test-agent not found in items")
	}

	if testAgent.Fields["app_id"] != "123456" {
		t.Errorf("expected app_id=123456, got %s", testAgent.Fields["app_id"])
	}
	if testAgent.Fields["installation_id"] != "78901234" {
		t.Errorf("expected installation_id=78901234, got %s", testAgent.Fields["installation_id"])
	}
	if testAgent.Fields["identity_name"] != "Test Agent" {
		t.Errorf("expected identity_name=Test Agent, got %s", testAgent.Fields["identity_name"])
	}
	if testAgent.Fields["identity_email"] != "test@example.com" {
		t.Errorf("expected identity_email=test@example.com, got %s", testAgent.Fields["identity_email"])
	}
}

func TestVaultBackend_GetItems_SkipsDirectories(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/secret/metadata/agents", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"keys": []string{"test-agent", "subdirectory/"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/v1/secret/data/agents/test-agent", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"data": map[string]interface{}{
					"app_id": "123",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/v1/auth/token/lookup-self", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"data": map[string]interface{}{"id": "test-token"}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	os.Setenv("VAULT_TOKEN", "test-token")
	defer os.Unsetenv("VAULT_TOKEN")

	backend, err := NewVaultBackend(server.URL, "secret")
	if err != nil {
		t.Fatalf("NewVaultBackend failed: %v", err)
	}

	items, err := backend.GetItems(context.Background(), "agents")
	if err != nil {
		t.Fatalf("GetItems failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("expected 1 item (directory skipped), got %d", len(items))
	}
	if items[0].Name != "test-agent" {
		t.Errorf("expected item name=test-agent, got %s", items[0].Name)
	}
}

func TestVaultBackend_GetAttachment_ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/token/lookup-self", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"data": map[string]interface{}{"id": "test-token"}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	os.Setenv("VAULT_TOKEN", "test-token")
	defer os.Unsetenv("VAULT_TOKEN")

	backend, err := NewVaultBackend(server.URL, "secret")
	if err != nil {
		t.Fatalf("NewVaultBackend failed: %v", err)
	}

	_, err = backend.GetAttachment(context.Background(), "item-id", "file.pem")
	if err != ErrAttachmentsNotSupported {
		t.Errorf("expected ErrAttachmentsNotSupported, got %v", err)
	}
}

func TestVaultBackend_Close(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/token/lookup-self", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"data": map[string]interface{}{"id": "test-token"}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	os.Setenv("VAULT_TOKEN", "test-token")
	defer os.Unsetenv("VAULT_TOKEN")

	backend, err := NewVaultBackend(server.URL, "secret")
	if err != nil {
		t.Fatalf("NewVaultBackend failed: %v", err)
	}

	if err := backend.Close(context.Background()); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestVaultBackend_Name(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/token/lookup-self", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"data": map[string]interface{}{"id": "test-token"}}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	os.Setenv("VAULT_TOKEN", "test-token")
	defer os.Unsetenv("VAULT_TOKEN")

	backend, err := NewVaultBackend(server.URL, "secret")
	if err != nil {
		t.Fatalf("NewVaultBackend failed: %v", err)
	}

	if backend.Name() != "vault" {
		t.Errorf("expected name=vault, got %s", backend.Name())
	}
}

func TestNewVaultBackend_MissingToken(t *testing.T) {
	os.Unsetenv("VAULT_TOKEN")
	os.Unsetenv("VAULT_TOKEN_FILE")

	_, err := NewVaultBackend("http://localhost:8200", "secret")
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
}
