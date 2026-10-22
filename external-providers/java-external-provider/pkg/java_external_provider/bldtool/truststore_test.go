package bldtool

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
)

// writeSelfSignedCA generates a self-signed CA certificate and returns the path to a PEM
// file containing it (optionally repeated, to exercise multi-cert bundles).
func writeSelfSignedCA(t *testing.T, dir string, count int) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Konveyor Test Internal CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	block := &pem.Block{Type: "CERTIFICATE", Bytes: der}

	pemPath := filepath.Join(dir, "ca.pem")
	f, err := os.Create(pemPath)
	if err != nil {
		t.Fatalf("creating pem file: %v", err)
	}
	defer f.Close()
	for i := 0; i < count; i++ {
		if err := pem.Encode(f, block); err != nil {
			t.Fatalf("encoding pem: %v", err)
		}
	}
	return pemPath
}

// requireKeytool skips the test when a usable JDK (JAVA_HOME + cacerts + keytool) is not
// available, so these tests pass in environments without a Java toolchain.
func requireKeytool(t *testing.T) {
	t.Helper()
	if os.Getenv("JAVA_HOME") == "" {
		t.Skip("JAVA_HOME not set; skipping truststore test")
	}
	if _, err := defaultCacertsPath(); err != nil {
		t.Skipf("JDK cacerts not available: %v", err)
	}
	if _, err := exec.LookPath(keytoolPath()); err != nil {
		if _, statErr := os.Stat(keytoolPath()); statErr != nil {
			t.Skip("keytool not available; skipping truststore test")
		}
	}
}

func TestBuildTrustStore(t *testing.T) {
	requireKeytool(t)

	for _, tc := range []struct {
		name  string
		certs int
	}{
		{name: "single cert", certs: 1},
		{name: "multi-cert bundle", certs: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			caPath := writeSelfSignedCA(t, dir, tc.certs)
			destDir := filepath.Join(dir, "ts")

			tsPath, err := BuildTrustStore(caPath, destDir, testr.New(t))
			if err != nil {
				t.Fatalf("BuildTrustStore failed: %v", err)
			}
			if _, err := os.Stat(tsPath); err != nil {
				t.Fatalf("expected truststore at %s: %v", tsPath, err)
			}

			// The custom CA aliases must be present...
			out, err := exec.Command(keytoolPath(),
				"-list", "-keystore", tsPath, "-storepass", "changeit").CombinedOutput()
			if err != nil {
				t.Fatalf("keytool -list failed: %v, output: %s", err, string(out))
			}
			listing := string(out)
			for i := 0; i < tc.certs; i++ {
				alias := "konveyor-ca-" + string(rune('0'+i))
				if !strings.Contains(listing, alias) {
					t.Errorf("expected alias %q in truststore, got:\n%s", alias, listing)
				}
			}
			// ...alongside the JDK defaults it was seeded from, so public CAs stay trusted.
			if !strings.Contains(listing, "trustedCertEntry") {
				t.Errorf("expected seeded JDK cacerts entries in truststore, got:\n%s", listing)
			}
		})
	}
}

func TestBuildTrustStoreNoCerts(t *testing.T) {
	requireKeytool(t)
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.pem")
	if err := os.WriteFile(empty, []byte("not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildTrustStore(empty, filepath.Join(dir, "ts"), testr.New(t)); err == nil {
		t.Error("expected error for a bundle with no certificates, got nil")
	}
}
