package server

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

// LoadTLSConfig returns a TLS config for the gRPC and HTTP servers.
//
//   - certFile+keyFile set: load that key pair (real / LE / internal CA cert).
//   - otherwise selfSigned: generate an ECDSA self-signed cert covering hosts,
//     persist it under dir so it is stable across restarts (agents can pin it),
//     and reuse it on later starts.
//   - neither: returns (nil, nil) — the caller then serves plaintext.
func LoadTLSConfig(certFile, keyFile string, selfSigned bool, hosts []string, dir string) (*tls.Config, error) {
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("loading TLS key pair: %w", err)
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
	}
	if !selfSigned {
		return nil, nil
	}

	certPath := filepath.Join(dir, "netlama-selfsigned.pem")
	keyPath := filepath.Join(dir, "netlama-selfsigned.key")

	// Reuse a previously generated cert so its identity is stable.
	if cert, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
	}

	certPEM, keyPEM, err := generateSelfSigned(hosts)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, fmt.Errorf("writing cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("writing key: %w", err)
	}
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, nil
}

// SelfSignedCertPath is where a generated self-signed cert is stored, so it
// can be copied to agents as their CA.
func SelfSignedCertPath(dir string) string {
	return filepath.Join(dir, "netlama-selfsigned.pem")
}

func generateSelfSigned(hosts []string) (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "net-lama"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	return certPEM, keyPEM, nil
}
