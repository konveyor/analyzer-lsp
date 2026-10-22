package bldtool

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/external-providers/java-external-provider/pkg/java_external_provider/dependency"
)

// BuildTrustStore creates a Java truststore that trusts the certificate(s) in the given
// PEM bundle in addition to the JDK's default trusted CAs, and returns its path.
//
// It seeds the store from a copy of the JDK cacerts so that public CAs (e.g. Maven
// Central over HTTPS) remain trusted, then imports every certificate from caCertPath into
// the copy with keytool. The resulting store is written under destDir, which must be
// writable, and is referenced via dependency.MavenTrustStoreArgs for both the provider's
// mvn subprocesses and the JDTLS JVM.
//
// This is the analyzer side of the hub Identity.CaCert field (tackle2-hub#1141): an admin
// attaches an internal CA to the Maven identity, the addon delivers it to the pod as a PEM
// file, and this builds the truststore the JVM actually consults. Unlike the "insecure"
// escape hatch, TLS verification stays on.
func BuildTrustStore(caCertPath string, destDir string, log logr.Logger) (string, error) {
	pemBytes, err := os.ReadFile(caCertPath)
	if err != nil {
		return "", fmt.Errorf("failed reading CA certificate bundle %s: %w", caCertPath, err)
	}

	sourceCacerts, err := defaultCacertsPath()
	if err != nil {
		return "", err
	}

	if err = os.MkdirAll(destDir, dependency.DirPermRWX); err != nil {
		return "", fmt.Errorf("failed creating truststore directory %s: %w", destDir, err)
	}
	trustStorePath := filepath.Join(destDir, "truststore.jks")
	if err = dependency.CopyFile(sourceCacerts, trustStorePath); err != nil {
		return "", fmt.Errorf("failed copying JDK cacerts from %s: %w", sourceCacerts, err)
	}

	// A PEM bundle may hold several certificates, but keytool -importcert only imports the
	// first entry of a bundle, so decode and import each certificate individually.
	var imported int
	rest := pemBytes
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		if _, err = x509.ParseCertificate(block.Bytes); err != nil {
			return "", fmt.Errorf("invalid certificate in CA bundle %s: %w", caCertPath, err)
		}
		if err = importCert(trustStorePath, pem.EncodeToMemory(block), imported, log); err != nil {
			return "", err
		}
		imported++
	}
	if imported == 0 {
		return "", fmt.Errorf("no PEM certificates found in CA bundle %s", caCertPath)
	}

	log.Info("built Maven/JDTLS truststore from custom CA bundle", "path", trustStorePath, "certificates", imported)
	return trustStorePath, nil
}

// importCert imports a single PEM-encoded certificate into the truststore at trustStorePath
// under a unique alias.
func importCert(trustStorePath string, certPEM []byte, index int, log logr.Logger) error {
	certFile, err := os.CreateTemp("", "konveyor-ca-*.pem")
	if err != nil {
		return fmt.Errorf("failed creating temp cert file: %w", err)
	}
	defer os.Remove(certFile.Name())
	if _, err = certFile.Write(certPEM); err != nil {
		certFile.Close()
		return fmt.Errorf("failed writing temp cert file: %w", err)
	}
	certFile.Close()

	alias := fmt.Sprintf("konveyor-ca-%d", index)
	cmd := exec.Command(keytoolPath(),
		"-importcert", "-noprompt", "-trustcacerts",
		"-keystore", trustStorePath,
		"-storepass", dependency.TrustStorePassword,
		"-alias", alias,
		"-file", certFile.Name(),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("keytool failed importing CA %q into truststore: %w, output: %s", alias, err, string(out))
	}
	log.V(5).Info("imported CA certificate into truststore", "alias", alias)
	return nil
}

// defaultCacertsPath returns the path to the JDK's default cacerts truststore, which the
// custom truststore is seeded from so public CAs remain trusted.
func defaultCacertsPath() (string, error) {
	javaHome := os.Getenv("JAVA_HOME")
	if javaHome == "" {
		return "", fmt.Errorf("JAVA_HOME is not set; cannot locate JDK cacerts to build truststore")
	}
	cacerts := filepath.Join(javaHome, "lib", "security", "cacerts")
	if _, err := os.Stat(cacerts); err != nil {
		return "", fmt.Errorf("JDK cacerts not found at %s: %w", cacerts, err)
	}
	return cacerts, nil
}

// keytoolPath returns the keytool executable, preferring $JAVA_HOME/bin/keytool and
// falling back to keytool on PATH.
func keytoolPath() string {
	if javaHome := os.Getenv("JAVA_HOME"); javaHome != "" {
		kt := filepath.Join(javaHome, "bin", "keytool")
		if _, err := os.Stat(kt); err == nil {
			return kt
		}
	}
	return "keytool"
}
