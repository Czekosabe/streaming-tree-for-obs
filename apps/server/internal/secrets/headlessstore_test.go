package secrets

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testMasterKey returns a fixed, obviously-fake 32-byte key - never a
// real operator credential, only ever used inside this test file.
func testMasterKey(t *testing.T, seed byte) []byte {
	t.Helper()
	key := make([]byte, masterKeyLength)
	for i := range key {
		key[i] = seed
	}
	return key
}

func newTestStore(t *testing.T, key []byte) (*HeadlessStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")
	store, err := NewHeadlessStore(path, key)
	if err != nil {
		t.Fatalf("NewHeadlessStore() error = %v", err)
	}
	return store, path
}

func TestHeadlessStoreWriteRead(t *testing.T) {
	store, _ := newTestStore(t, testMasterKey(t, 1))
	ctx := context.Background()

	if err := store.Set(ctx, "k1", []byte("hello")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, err := store.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("Get() = %q, want %q", got, "hello")
	}
}

func TestHeadlessStoreOverwrite(t *testing.T) {
	store, _ := newTestStore(t, testMasterKey(t, 2))
	ctx := context.Background()

	if err := store.Set(ctx, "k1", []byte("first")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Set(ctx, "k1", []byte("second")); err != nil {
		t.Fatalf("Set() overwrite error = %v", err)
	}
	got, err := store.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got) != "second" {
		t.Fatalf("Get() = %q, want %q", got, "second")
	}
}

func TestHeadlessStoreDelete(t *testing.T) {
	store, _ := newTestStore(t, testMasterKey(t, 3))
	ctx := context.Background()

	if err := store.Set(ctx, "k1", []byte("value")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(ctx, "k1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after Delete() error = %v, want ErrNotFound", err)
	}
	if err := store.Delete(ctx, "k1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() of an already-deleted key error = %v, want ErrNotFound", err)
	}
}

func TestHeadlessStoreExists(t *testing.T) {
	store, _ := newTestStore(t, testMasterKey(t, 4))
	ctx := context.Background()

	exists, err := store.Exists(ctx, "missing")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if exists {
		t.Fatal("Exists() = true for a key never set")
	}

	if err := store.Set(ctx, "present", []byte("v")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	exists, err = store.Exists(ctx, "present")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Fatal("Exists() = false for a key that was set")
	}
}

func TestHeadlessStoreGetMissingReturnsErrNotFound(t *testing.T) {
	store, _ := newTestStore(t, testMasterKey(t, 5))
	if _, err := store.Get(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestHeadlessStorePersistsAcrossReopen(t *testing.T) {
	key := testMasterKey(t, 6)
	store, path := newTestStore(t, key)
	ctx := context.Background()

	if err := store.Set(ctx, "k1", []byte("persisted")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	reopened, err := NewHeadlessStore(path, key)
	if err != nil {
		t.Fatalf("NewHeadlessStore() (reopen) error = %v", err)
	}
	got, err := reopened.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get() after reopen error = %v", err)
	}
	if string(got) != "persisted" {
		t.Fatalf("Get() after reopen = %q, want %q", got, "persisted")
	}
}

func TestHeadlessStoreIndependentKeysProduceDifferentCiphertext(t *testing.T) {
	store, path := newTestStore(t, testMasterKey(t, 7))
	ctx := context.Background()

	if err := store.Set(ctx, "key-a", []byte("same-plaintext")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := store.Set(ctx, "key-b", []byte("same-plaintext")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	entries := readRawEntries(t, path)
	if entries["key-a"] == entries["key-b"] {
		t.Fatal("identical plaintext under two different keys produced identical ciphertext")
	}
}

func TestHeadlessStoreRandomNonceChangesCiphertextForIdenticalPlaintext(t *testing.T) {
	store, path := newTestStore(t, testMasterKey(t, 8))
	ctx := context.Background()

	if err := store.Set(ctx, "k1", []byte("same-value")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	first := readRawEntries(t, path)["k1"]

	if err := store.Set(ctx, "k1", []byte("same-value")); err != nil {
		t.Fatalf("Set() (second write) error = %v", err)
	}
	second := readRawEntries(t, path)["k1"]

	if first == second {
		t.Fatal("re-encrypting identical plaintext under the same key produced identical ciphertext (nonce reuse)")
	}
}

func TestHeadlessStoreWrongMasterKeyRejected(t *testing.T) {
	key := testMasterKey(t, 9)
	store, path := newTestStore(t, key)
	ctx := context.Background()
	if err := store.Set(ctx, "k1", []byte("secret-value")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	wrongKey := testMasterKey(t, 10)
	reopened, err := NewHeadlessStore(path, wrongKey)
	if err != nil {
		t.Fatalf("NewHeadlessStore() error = %v", err)
	}
	if _, err := reopened.Get(ctx, "k1"); !errors.Is(err, ErrFailure) {
		t.Fatalf("Get() with the wrong master key error = %v, want ErrFailure", err)
	}
}

func TestHeadlessStoreTamperedCiphertextRejected(t *testing.T) {
	key := testMasterKey(t, 11)
	store, path := newTestStore(t, key)
	ctx := context.Background()
	if err := store.Set(ctx, "k1", []byte("secret-value")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	tamperEntry(t, path, "k1", func(sealed []byte) []byte {
		tampered := append([]byte(nil), sealed...)
		tampered[len(tampered)-1] ^= 0xFF // flip a bit inside the GCM tag/ciphertext
		return tampered
	})

	reopened, err := NewHeadlessStore(path, key)
	if err != nil {
		t.Fatalf("NewHeadlessStore() error = %v", err)
	}
	if _, err := reopened.Get(ctx, "k1"); !errors.Is(err, ErrFailure) {
		t.Fatalf("Get() with tampered ciphertext error = %v, want ErrFailure", err)
	}
}

func TestHeadlessStoreTruncatedCiphertextRejected(t *testing.T) {
	key := testMasterKey(t, 12)
	store, path := newTestStore(t, key)
	ctx := context.Background()
	if err := store.Set(ctx, "k1", []byte("secret-value")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	tamperEntry(t, path, "k1", func(sealed []byte) []byte {
		return sealed[:5] // shorter than the nonce alone
	})

	reopened, err := NewHeadlessStore(path, key)
	if err != nil {
		t.Fatalf("NewHeadlessStore() error = %v", err)
	}
	if _, err := reopened.Get(ctx, "k1"); !errors.Is(err, ErrFailure) {
		t.Fatalf("Get() with truncated ciphertext error = %v, want ErrFailure", err)
	}
}

func TestHeadlessStoreMalformedNonceEncodingRejected(t *testing.T) {
	key := testMasterKey(t, 13)
	store, path := newTestStore(t, key)
	ctx := context.Background()
	if err := store.Set(ctx, "k1", []byte("secret-value")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading store file: %v", err)
	}
	var data headlessStoreFile
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshalling store file: %v", err)
	}
	data.Entries["k1"] = "not-valid-base64!!!"
	overwrite(t, path, data)

	reopened, err := NewHeadlessStore(path, key)
	if err != nil {
		t.Fatalf("NewHeadlessStore() error = %v", err)
	}
	if _, err := reopened.Get(ctx, "k1"); !errors.Is(err, ErrFailure) {
		t.Fatalf("Get() with malformed base64 error = %v, want ErrFailure", err)
	}
}

func TestHeadlessStoreUnknownEnvelopeVersionRejected(t *testing.T) {
	key := testMasterKey(t, 14)
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	future := headlessStoreFile{Format: headlessStoreFormat, Version: headlessStoreVersion + 1, Entries: map[string]string{}}
	overwrite(t, path, future)

	store, err := NewHeadlessStore(path, key)
	if err != nil {
		t.Fatalf("NewHeadlessStore() error = %v", err)
	}
	if _, err := store.Exists(context.Background(), "anything"); !errors.Is(err, ErrFailure) {
		t.Fatalf("Exists() against an unknown envelope version error = %v, want ErrFailure", err)
	}
}

func TestHeadlessStoreUnknownFormatRejected(t *testing.T) {
	key := testMasterKey(t, 15)
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")

	wrongFormat := headlessStoreFile{Format: "something-else", Version: headlessStoreVersion, Entries: map[string]string{}}
	overwrite(t, path, wrongFormat)

	store, err := NewHeadlessStore(path, key)
	if err != nil {
		t.Fatalf("NewHeadlessStore() error = %v", err)
	}
	if _, err := store.Exists(context.Background(), "anything"); !errors.Is(err, ErrFailure) {
		t.Fatalf("Exists() against an unrecognized format error = %v, want ErrFailure", err)
	}
}

func TestHeadlessStoreOversizedValueAccepted(t *testing.T) {
	// Not a hard product limit - AES-GCM handles large inputs fine
	// (well under its own 64GiB theoretical bound) - this proves a
	// realistically large stream key/OAuth token bundle round-trips
	// correctly rather than being silently truncated.
	store, _ := newTestStore(t, testMasterKey(t, 16))
	big := make([]byte, 64*1024)
	if _, err := rand.Read(big); err != nil {
		t.Fatalf("generating test payload: %v", err)
	}
	ctx := context.Background()
	if err := store.Set(ctx, "big", big); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	got, err := store.Get(ctx, "big")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !bytes.Equal(got, big) {
		t.Fatal("large value did not round-trip correctly")
	}
}

func TestHeadlessStorePlaintextAbsentFromBackingFile(t *testing.T) {
	store, path := newTestStore(t, testMasterKey(t, 17))
	const secretMarker = "sk_live_super_secret_marker_value"
	if err := store.Set(context.Background(), "k1", []byte(secretMarker)); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading store file: %v", err)
	}
	if strings.Contains(string(raw), secretMarker) {
		t.Fatal("plaintext secret value found in the backing file")
	}
}

func TestNewHeadlessStoreRejectsWrongKeyLength(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.json")
	for _, n := range []int{0, 16, 31, 33, 64} {
		if _, err := NewHeadlessStore(path, make([]byte, n)); !errors.Is(err, ErrUnavailable) {
			t.Errorf("NewHeadlessStore() with a %d-byte key error = %v, want ErrUnavailable", n, err)
		}
	}
}

func TestLoadHeadlessMasterKeyMissingCredentialsDirectory(t *testing.T) {
	t.Setenv("CREDENTIALS_DIRECTORY", "")
	if _, err := LoadHeadlessMasterKey(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("LoadHeadlessMasterKey() error = %v, want ErrUnavailable", err)
	}
}

func TestLoadHeadlessMasterKeyMissingFile(t *testing.T) {
	t.Setenv("CREDENTIALS_DIRECTORY", t.TempDir())
	if _, err := LoadHeadlessMasterKey(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("LoadHeadlessMasterKey() error = %v, want ErrUnavailable", err)
	}
}

func TestLoadHeadlessMasterKeyWrongLength(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CREDENTIALS_DIRECTORY", dir)
	if err := os.WriteFile(filepath.Join(dir, HeadlessMasterKeyCredentialName), []byte("too-short"), 0o600); err != nil {
		t.Fatalf("writing fake credential: %v", err)
	}
	if _, err := LoadHeadlessMasterKey(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("LoadHeadlessMasterKey() error = %v, want ErrUnavailable", err)
	}
}

func TestLoadHeadlessMasterKeyValid(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CREDENTIALS_DIRECTORY", dir)
	want := testMasterKey(t, 42)
	if err := os.WriteFile(filepath.Join(dir, HeadlessMasterKeyCredentialName), want, 0o600); err != nil {
		t.Fatalf("writing fake credential: %v", err)
	}
	got, err := LoadHeadlessMasterKey()
	if err != nil {
		t.Fatalf("LoadHeadlessMasterKey() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("LoadHeadlessMasterKey() returned unexpected bytes")
	}
}

func TestHeadlessStoreConcurrentAccessConsistent(t *testing.T) {
	// The existing SecretStore interface offers no explicit concurrency
	// contract beyond "implementations must be safe for concurrent
	// use" (store.go's own doc comment) - this proves HeadlessStore
	// meets that bar: N concurrent Set calls to independent keys must
	// all be observable afterward, none silently lost to a lost update
	// in the read-modify-write cycle.
	store, _ := newTestStore(t, testMasterKey(t, 18))
	ctx := context.Background()
	const n = 20
	done := make(chan error, n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			key := "k" + string(rune('a'+i))
			done <- store.Set(ctx, key, []byte{byte(i)})
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent Set() error = %v", err)
		}
	}
	for i := 0; i < n; i++ {
		key := "k" + string(rune('a'+i))
		got, err := store.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get(%q) error = %v", key, err)
		}
		if len(got) != 1 || got[0] != byte(i) {
			t.Fatalf("Get(%q) = %v, want [%d]", key, got, i)
		}
	}
}

// --- test helpers ----------------------------------------------------

func readRawEntries(t *testing.T, path string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading store file: %v", err)
	}
	var data headlessStoreFile
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshalling store file: %v", err)
	}
	return data.Entries
}

func overwrite(t *testing.T, path string, data headlessStoreFile) {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshalling test fixture: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("writing test fixture: %v", err)
	}
}

func tamperEntry(t *testing.T, path, key string, mutate func([]byte) []byte) {
	t.Helper()
	data := headlessStoreFile{}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading store file: %v", err)
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshalling store file: %v", err)
	}
	sealed, err := base64.StdEncoding.DecodeString(data.Entries[key])
	if err != nil {
		t.Fatalf("decoding entry: %v", err)
	}
	data.Entries[key] = base64.StdEncoding.EncodeToString(mutate(sealed))
	overwrite(t, path, data)
}
