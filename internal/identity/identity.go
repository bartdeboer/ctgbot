package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	CertFile = "identity.crt"
	KeyFile  = "identity.key"
)

type Identity struct {
	ID             string
	DisplayName    string
	CertificatePEM string
	Fingerprint    string
	Certificate    *x509.Certificate
	TLSCertificate tls.Certificate
}

type Manager struct {
	Dir         string
	DisplayName string
}

func NewManager(dir string, displayName string) *Manager {
	return &Manager{Dir: strings.TrimSpace(dir), DisplayName: strings.TrimSpace(displayName)}
}

func (m *Manager) Ensure() (Identity, error) {
	if m == nil || strings.TrimSpace(m.Dir) == "" {
		return Identity{}, fmt.Errorf("missing identity directory")
	}
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		return Identity{}, err
	}
	certPath := filepath.Join(m.Dir, CertFile)
	keyPath := filepath.Join(m.Dir, KeyFile)
	if !fileExists(certPath) || !fileExists(keyPath) {
		if err := generateSelfSignedIdentity(certPath, keyPath, m.displayName()); err != nil {
			return Identity{}, err
		}
	}
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return Identity{}, err
	}
	if len(pair.Certificate) == 0 {
		return Identity{}, fmt.Errorf("identity certificate has no DER data")
	}
	cert, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return Identity{}, err
	}
	pemBytes, err := os.ReadFile(certPath)
	if err != nil {
		return Identity{}, err
	}
	fingerprint := Fingerprint(cert)
	return Identity{
		ID:             fingerprint,
		DisplayName:    m.displayName(),
		CertificatePEM: strings.TrimSpace(string(pemBytes)),
		Fingerprint:    fingerprint,
		Certificate:    cert,
		TLSCertificate: pair,
	}, nil
}

func (m *Manager) displayName() string {
	if m != nil && strings.TrimSpace(m.DisplayName) != "" {
		return strings.TrimSpace(m.DisplayName)
	}
	host, err := os.Hostname()
	if err == nil && strings.TrimSpace(host) != "" {
		return "ctgbot@" + strings.TrimSpace(host)
	}
	return "ctgbot"
}

func Fingerprint(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	sum := sha256.Sum256(cert.Raw)
	return "SHA256:" + hex.EncodeToString(sum[:])
}

func PairingCode(state tls.ConnectionState) (string, error) {
	material, err := state.ExportKeyingMaterial("ctgbot-pairing-v1", nil, 6)
	if err != nil {
		return "", err
	}
	if len(material) < 6 {
		return "", fmt.Errorf("pairing material too short")
	}
	left := int(material[0])<<16 | int(material[1])<<8 | int(material[2])
	right := int(material[3])<<16 | int(material[4])<<8 | int(material[5])
	return fmt.Sprintf("%03d-%03d", left%1000, right%1000), nil
}

func TLSConfig(id Identity) *tls.Config {
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{id.TLSCertificate},
		ClientAuth:   tls.RequestClientCert,
	}
}

func ClientTLSConfig(id Identity, insecureSkipVerify bool) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		Certificates:       []tls.Certificate{id.TLSCertificate},
		InsecureSkipVerify: insecureSkipVerify, // Pairing uses the human-verified TLS exporter code as the trust check.
	}
}

func ParseCertificatePEM(value string) (*x509.Certificate, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(value)))
	if block == nil {
		return nil, fmt.Errorf("decode certificate PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

func generateSelfSignedIdentity(certPath, keyPath, displayName string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject: pkix.Name{
			CommonName: displayName,
		},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	if err := writeCert(certPath, der); err != nil {
		return err
	}
	return writeKey(keyPath, key)
}

func writeCert(path string, der []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	return pem.Encode(file, &pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func writeKey(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return pem.Encode(file, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
