package content

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

// CertInfo holds details extracted from an X.509 TLS certificate.
type CertInfo struct {
	Host               string    `json:"host"`
	Subject            string    `json:"subject"`
	Issuer             string    `json:"issuer"`
	NotBefore          time.Time `json:"not_before"`
	NotAfter           time.Time `json:"not_after"`
	Fingerprint        string    `json:"fingerprint"`
	DNSNames           []string  `json:"dns_names"`
	IsExpired          bool      `json:"is_expired"`
	IsSelfSigned       bool      `json:"is_self_signed"`
	SerialNumber       string    `json:"serial_number"`
	SignatureAlgorithm string    `json:"signature_algorithm"`
}

// InspectCert dials host over TLS and returns certificate details.
// InsecureSkipVerify is enabled so that expired or self-signed certs can still
// be inspected without causing a connection failure.
func InspectCert(host string) (*CertInfo, error) {
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}

	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	cfg := &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // intentional — we inspect, not validate
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", host, cfg)
	if err != nil {
		return nil, fmt.Errorf("tls dial %s: %w", host, err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no peer certificates returned from %s", host)
	}

	cert := certs[0]

	sum := sha256.Sum256(cert.Raw)
	fingerprint := hex.EncodeToString(sum[:])

	info := &CertInfo{
		Host:               host,
		Subject:            cert.Subject.CommonName,
		Issuer:             cert.Issuer.CommonName,
		NotBefore:          cert.NotBefore,
		NotAfter:           cert.NotAfter,
		Fingerprint:        fingerprint,
		DNSNames:           cert.DNSNames,
		IsExpired:          cert.NotAfter.Before(time.Now()),
		IsSelfSigned:       cert.Issuer.String() == cert.Subject.String(),
		SerialNumber:       cert.SerialNumber.String(),
		SignatureAlgorithm: cert.SignatureAlgorithm.String(),
	}

	return info, nil
}

// ToJSON marshals CertInfo to JSON.
func (ci *CertInfo) ToJSON() ([]byte, error) {
	return json.Marshal(ci)
}
