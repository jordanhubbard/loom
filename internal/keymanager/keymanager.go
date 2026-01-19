package keymanager

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/pbkdf2"
)

// KeyEntry represents an encrypted credential entry
type KeyEntry struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	EncryptedData string    `json:"encrypted_data"` // Base64 encoded encrypted key
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// KeyStore represents the encrypted key storage
type KeyStore struct {
	Keys map[string]*KeyEntry `json:"keys"`
}

// KeyManager manages secure storage and retrieval of provider credentials
type KeyManager struct {
	storePath string
	password  []byte
	store     *KeyStore
	mu        sync.RWMutex
	unlocked  bool
}

const (
	saltSize   = 32
	keySize    = 32
	iterations = 100000
)

// NewKeyManager creates a new key manager instance
func NewKeyManager(storePath string) *KeyManager {
	return &KeyManager{
		storePath: storePath,
		store: &KeyStore{
			Keys: make(map[string]*KeyEntry),
		},
	}
}

// Unlock unlocks the key store with the provided password
// The password is derived into an encryption key but not stored
func (km *KeyManager) Unlock(password string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Derive encryption key from password using PBKDF2
	km.password = []byte(password)

	// Try to load existing store
	if err := km.loadStore(); err != nil {
		// If store doesn't exist, initialize a new one
		if os.IsNotExist(err) {
			km.store = &KeyStore{
				Keys: make(map[string]*KeyEntry),
			}
			// Save the empty store
			if err := km.saveStore(); err != nil {
				return fmt.Errorf("failed to initialize key store: %w", err)
			}
		} else {
			return fmt.Errorf("failed to unlock key store: %w", err)
		}
	}

	km.unlocked = true
	return nil
}

// IsUnlocked returns whether the key store is unlocked
func (km *KeyManager) IsUnlocked() bool {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.unlocked
}

// StoreKey stores an encrypted credential
func (km *KeyManager) StoreKey(id, name, description, key string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	if !km.unlocked {
		return errors.New("key store is locked")
	}

	// Encrypt the key
	encryptedData, err := km.encrypt([]byte(key))
	if err != nil {
		return fmt.Errorf("failed to encrypt key: %w", err)
	}

	// Store the encrypted key
	km.store.Keys[id] = &KeyEntry{
		ID:            id,
		Name:          name,
		Description:   description,
		EncryptedData: base64.StdEncoding.EncodeToString(encryptedData),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	// Persist to disk
	if err := km.saveStore(); err != nil {
		return fmt.Errorf("failed to save key store: %w", err)
	}

	return nil
}

// GetKey retrieves and decrypts a credential
func (km *KeyManager) GetKey(id string) (string, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if !km.unlocked {
		return "", errors.New("key store is locked")
	}

	entry, exists := km.store.Keys[id]
	if !exists {
		return "", fmt.Errorf("key not found: %s", id)
	}

	// Decode and decrypt
	encryptedData, err := base64.StdEncoding.DecodeString(entry.EncryptedData)
	if err != nil {
		return "", fmt.Errorf("failed to decode key: %w", err)
	}

	decryptedData, err := km.decrypt(encryptedData)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt key: %w", err)
	}

	return string(decryptedData), nil
}

// DeleteKey removes a credential from the store
func (km *KeyManager) DeleteKey(id string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	if !km.unlocked {
		return errors.New("key store is locked")
	}

	delete(km.store.Keys, id)

	// Persist to disk
	if err := km.saveStore(); err != nil {
		return fmt.Errorf("failed to save key store: %w", err)
	}

	return nil
}

// ListKeys returns a list of all key entries (without decrypted data)
func (km *KeyManager) ListKeys() ([]*KeyEntry, error) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if !km.unlocked {
		return nil, errors.New("key store is locked")
	}

	keys := make([]*KeyEntry, 0, len(km.store.Keys))
	for _, entry := range km.store.Keys {
		// Return a copy without the encrypted data
		keys = append(keys, &KeyEntry{
			ID:          entry.ID,
			Name:        entry.Name,
			Description: entry.Description,
			CreatedAt:   entry.CreatedAt,
			UpdatedAt:   entry.UpdatedAt,
		})
	}

	return keys, nil
}

// Lock locks the key store and clears the password from memory
func (km *KeyManager) Lock() {
	km.mu.Lock()
	defer km.mu.Unlock()

	// Clear password from memory
	if km.password != nil {
		for i := range km.password {
			km.password[i] = 0
		}
		km.password = nil
	}

	km.unlocked = false
}

// encrypt encrypts data using AES-GCM
func (km *KeyManager) encrypt(plaintext []byte) ([]byte, error) {
	// Generate salt
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}

	// Derive key from password
	key := pbkdf2.Key(km.password, salt, iterations, keySize, sha256.New)

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Prepend salt and nonce to ciphertext
	result := make([]byte, 0, saltSize+len(nonce)+len(ciphertext))
	result = append(result, salt...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)

	return result, nil
}

// decrypt decrypts data using AES-GCM
func (km *KeyManager) decrypt(data []byte) ([]byte, error) {
	if len(data) < saltSize {
		return nil, errors.New("invalid encrypted data")
	}

	// Extract salt
	salt := data[:saltSize]
	data = data[saltSize:]

	// Derive key from password
	key := pbkdf2.Key(km.password, salt, iterations, keySize, sha256.New)

	// Create cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("invalid encrypted data")
	}

	// Extract nonce
	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	// Decrypt
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// loadStore loads the key store from disk
func (km *KeyManager) loadStore() error {
	data, err := os.ReadFile(km.storePath)
	if err != nil {
		return err
	}

	// The store file itself is not encrypted, only individual keys are
	// This allows us to see metadata without unlocking
	var store KeyStore
	if err := json.Unmarshal(data, &store); err != nil {
		return err
	}

	km.store = &store
	return nil
}

// saveStore saves the key store to disk
func (km *KeyManager) saveStore() error {
	data, err := json.MarshalIndent(km.store, "", "  ")
	if err != nil {
		return err
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(km.storePath), 0700); err != nil {
		return err
	}

	// Write with restricted permissions
	if err := os.WriteFile(km.storePath, data, 0600); err != nil {
		return err
	}

	return nil
}
