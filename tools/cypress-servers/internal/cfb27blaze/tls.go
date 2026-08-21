package cfb27blaze

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func newTLSConfig(mode string, extraIPs ...net.IP) (*tls.Config, error) {
	ca, caKey, err := loadOrCreateLocalCA()
	if err != nil {
		return nil, err
	}
	dnsNames := []string{
		"spring25.client.blazeredirector.ea.com",
		"gosca25.blazeredirector.ea.com",
		"spring25.gosredirector.ea.com",
		"spring18.gosredirector.ea.com",
		"gcs.ea.com",
		"collector.errors.ea.com",
		"a-collector.errors.ea.com",
		"freeform-river.data.ea.com",
		"update.layer.ea.com",
		"localhost",
	}
	// A joining player dials the host by IP, so the bind address must be a SAN
	// too. With only loopback here, their connection is offered a certificate
	// that names nothing they asked for.
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	for _, extra := range extraIPs {
		if extra != nil && !extra.IsUnspecified() {
			ips = append(ips, extra)
		}
	}
	certificate, err := issueServerCertificate(ca, caKey, dnsNames[0], dnsNames, ips)
	if err != nil {
		return nil, err
	}
	maxVersion := uint16(tls.VersionTLS13)
	nextProtos := []string{"h2", "http/1.1"}
	switch mode {
	case "", "tls13":
	case "tls12":
		maxVersion = tls.VersionTLS12
	case "tls12-noalpn":
		maxVersion = tls.VersionTLS12
		nextProtos = nil
	case "tls13-noalpn":
		nextProtos = nil
	default:
		return nil, fmt.Errorf("unsupported TLS mode %q", mode)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		// The offline server never authenticates clients by certificate. This is
		// intentionally explicit so redirected test clients are all accepted.
		ClientAuth: tls.NoClientCert,
		MinVersion: tls.VersionTLS12,
		// Allow TLS 1.3. The game's ProtoSSL client offers 1.3 (0x0304) first and, when
		// we forced a 1.2 downgrade, closed the connection with a clean EOF and no TLS
		// alert right after our ServerHello — the signature of a client that requires
		// 1.3. (The earlier belief that the BearSSL end_chain cert-bypass hook only runs
		// on the 1.2 path was moot: end_chain never fired on 1.2 either, because the
		// handshake died before certificate validation.) BearSSL supports TLS 1.3, so
		// let the client negotiate its preferred version.
		MaxVersion: maxVersion,
		NextProtos: nextProtos,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.CurveP384,
			tls.X25519,
		},
	}

	// Redirected endpoints can retain their original SNI. Mint a matching leaf
	// from the private local CA so hostname checks do not fail merely because a
	// newly observed test endpoint was not in the original static host list.
	var certificateMu sync.Mutex
	certificates := make(map[string]*tls.Certificate)
	config.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		serverName := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(hello.ServerName)), ".")
		if serverName == "" || len(serverName) > 253 || strings.ContainsRune(serverName, '\x00') {
			return &config.Certificates[0], nil
		}
		certificateMu.Lock()
		defer certificateMu.Unlock()
		if cached := certificates[serverName]; cached != nil {
			return cached, nil
		}
		var dynamic tls.Certificate
		var err error
		if ip := net.ParseIP(serverName); ip != nil {
			dynamic, err = issueServerCertificate(ca, caKey, serverName, nil, []net.IP{ip})
		} else {
			dynamic, err = issueServerCertificate(ca, caKey, serverName, []string{serverName}, nil)
		}
		if err != nil {
			return nil, err
		}
		certificates[serverName] = &dynamic
		return certificates[serverName], nil
	}
	return config, nil
}

func issueServerCertificate(ca *x509.Certificate, caKey *rsa.PrivateKey, commonName string, dnsNames []string, ipAddresses []net.IP) (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber: big.NewInt(now.UnixNano()),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.AddDate(0, 1, 0),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ipAddresses,
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, &template, ca, &key.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{
		Certificate: [][]byte{certificateDER, ca.Raw},
		PrivateKey:  key,
	}, nil
}

func loadOrCreateLocalCA() (*x509.Certificate, *rsa.PrivateKey, error) {
	dir, err := localCertDir()
	if err != nil {
		return nil, nil, err
	}
	certPath := filepath.Join(dir, "cypress-cfb27-local-ca.cer")
	keyPath := filepath.Join(dir, "cypress-cfb27-local-ca.key")
	if cert, key, err := loadLocalCA(certPath, keyPath); err == nil {
		return cert, key, nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, nil, err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber:          big.NewInt(now.UnixNano()),
		Subject:               pkix.Name{CommonName: "Cypress CFB27 Local Root CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), 0o644); err != nil {
		return nil, nil, err
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func loadLocalCA(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("decode CA certificate")
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("decode CA key")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func localCertDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "Cypress", "CFB27", "Private", "certs"), nil
}
