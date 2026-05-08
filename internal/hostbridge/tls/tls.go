package hostbridgetls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	CACertFile     = "ca.crt"
	CAKeyFile      = "ca.key"
	ServerCertFile = "server.crt"
	ServerKeyFile  = "server.key"
	ClientCertFile = "client.crt"
	ClientKeyFile  = "client.key"

	ServerName = "host.docker.internal"
)

func EnsureServerMaterials(root string) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	certPath := filepath.Join(root, CACertFile)
	keyPath := filepath.Join(root, CAKeyFile)

	caCert, caKey, err := ensureCA(certPath, keyPath)
	if err != nil {
		return err
	}

	serverCertPath := filepath.Join(root, ServerCertFile)
	serverKeyPath := filepath.Join(root, ServerKeyFile)
	if fileExists(serverCertPath) && fileExists(serverKeyPath) {
		return nil
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber: serialNumber(),
		Subject: pkix.Name{
			CommonName: "ctgbot-hostbridge-server",
		},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"host.docker.internal", "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	if err := writeCertPEM(serverCertPath, der); err != nil {
		return err
	}
	return writeECPrivateKeyPEM(serverKeyPath, serverKey)
}

func EnsureChatClientMaterials(serverRoot string, chatTLSDir string, commonName string) error {
	if err := EnsureServerMaterials(serverRoot); err != nil {
		return err
	}
	if err := os.MkdirAll(chatTLSDir, 0o700); err != nil {
		return err
	}

	caCertPath := filepath.Join(serverRoot, CACertFile)
	caKeyPath := filepath.Join(serverRoot, CAKeyFile)
	clientCertPath := filepath.Join(chatTLSDir, ClientCertFile)
	clientKeyPath := filepath.Join(chatTLSDir, ClientKeyFile)
	chatCACertPath := filepath.Join(chatTLSDir, CACertFile)

	if !fileExists(chatCACertPath) {
		if err := copyFile(caCertPath, chatCACertPath, 0o644); err != nil {
			return err
		}
	}
	if fileExists(clientCertPath) && fileExists(clientKeyPath) {
		return nil
	}

	caCert, caKey, err := loadCA(caCertPath, caKeyPath)
	if err != nil {
		return err
	}

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber: serialNumber(),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	if err := writeCertPEM(clientCertPath, der); err != nil {
		return err
	}
	return writeECPrivateKeyPEM(clientKeyPath, clientKey)
}

func LoadServerTLSConfig(root string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(filepath.Join(root, ServerCertFile), filepath.Join(root, ServerKeyFile))
	if err != nil {
		return nil, err
	}

	caPEM, err := os.ReadFile(filepath.Join(root, CACertFile))
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("append ca certs")
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}, nil
}

func LoadClientTLSConfig(dir string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, ClientCertFile), filepath.Join(dir, ClientKeyFile))
	if err != nil {
		return nil, err
	}
	caPEM, err := os.ReadFile(filepath.Join(dir, CACertFile))
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("append ca certs")
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   ServerName,
	}, nil
}

func ensureCA(certPath, keyPath string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	if fileExists(certPath) && fileExists(keyPath) {
		return loadCA(certPath, keyPath)
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber: serialNumber(),
		Subject: pkix.Name{
			CommonName: "ctgbot-hostbridge-ca",
		},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	if err := writeCertPEM(certPath, der); err != nil {
		return nil, nil, err
	}
	if err := writeECPrivateKeyPEM(keyPath, caKey); err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, caKey, nil
}

func loadCA(certPath, keyPath string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, err
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("decode ca cert pem")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("decode ca key pem")
	}
	keyAny, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, keyAny, nil
}

func writeCertPEM(path string, der []byte) error {
	return writePEM(path, &pem.Block{Type: "CERTIFICATE", Bytes: der}, 0o644)
}

func writeECPrivateKeyPEM(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	return writePEM(path, &pem.Block{Type: "EC PRIVATE KEY", Bytes: der}, 0o600)
}

func writePEM(path string, block *pem.Block, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := pem.Encode(f, block); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func serialNumber() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return big.NewInt(time.Now().UnixNano())
	}
	return n
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}
