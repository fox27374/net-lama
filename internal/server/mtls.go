package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// Mutual TLS for the agent control stream: the gRPC listener additionally
// requires a client certificate signed by a trusted CA, and the certificate
// CN must match the name of the agent the token resolves to. The web UI/API
// is unaffected (browsers keep plain server-auth TLS).

// AgentCAPath is where the built-in agent CA certificate is stored, so it
// can be copied or inspected.
func AgentCAPath(dir string) string {
	return filepath.Join(dir, "netlama-agent-ca.pem")
}

func agentCAKeyPath(dir string) string {
	return filepath.Join(dir, "netlama-agent-ca.key")
}

// ClientCAPool returns the CA pool the gRPC server trusts for agent client
// certificates: caFile if given (own/internal CA), otherwise the built-in
// agent CA under dir, generated on first use.
func ClientCAPool(caFile, dir string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if caFile != "" {
		pemData, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("reading mTLS client CA: %w", err)
		}
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("no certificates found in %s", caFile)
		}
		return pool, nil
	}
	caCert, _, err := loadOrCreateAgentCA(dir)
	if err != nil {
		return nil, err
	}
	pool.AddCert(caCert)
	return pool, nil
}

// IssueAgentCert creates a client certificate for the named agent, signed by
// the built-in agent CA (generated on first use), and persists the pair next
// to the CA. The CN is set to name — the server rejects the connection if it
// does not match the agent name the token resolves to.
func IssueAgentCert(dir, name string) (certPath, keyPath string, err error) {
	caCert, caKey, err := loadOrCreateAgentCA(dir)
	if err != nil {
		return "", "", err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, caCert, &key.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", err
	}

	certPath = filepath.Join(dir, "agent-"+name+".pem")
	keyPath = filepath.Join(dir, "agent-"+name+".key")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return "", "", fmt.Errorf("writing agent cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "", "", fmt.Errorf("writing agent key: %w", err)
	}
	return certPath, keyPath, nil
}

// PeerCertCN returns the CommonName of the verified client certificate on
// the connection behind ctx, or "" when there is none.
func PeerCertCN(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return ""
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return ""
	}
	if chains := tlsInfo.State.VerifiedChains; len(chains) > 0 && len(chains[0]) > 0 {
		return chains[0][0].Subject.CommonName
	}
	return ""
}

// loadOrCreateAgentCA returns the built-in CA used to sign agent client
// certificates, generating and persisting it under dir on first use.
func loadOrCreateAgentCA(dir string) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPath, keyPath := AgentCAPath(dir), agentCAKeyPath(dir)

	if pair, err := tls.LoadX509KeyPair(certPath, keyPath); err == nil {
		cert, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return nil, nil, fmt.Errorf("parsing agent CA: %w", err)
		}
		key, ok := pair.PrivateKey.(*ecdsa.PrivateKey)
		if !ok {
			return nil, nil, fmt.Errorf("agent CA key in %s is not ECDSA", keyPath)
		}
		return cert, key, nil
	}

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
		Subject:               pkix.Name{CommonName: "net-lama agent CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, nil, fmt.Errorf("writing agent CA cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, nil, fmt.Errorf("writing agent CA key: %w", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}
