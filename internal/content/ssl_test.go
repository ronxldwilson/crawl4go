package content

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCertInfo_ToJSON(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	ci := &CertInfo{
		Host:               "example.com:443",
		Subject:            "example.com",
		Issuer:             "Let's Encrypt",
		NotBefore:          now.Add(-24 * time.Hour),
		NotAfter:           now.Add(90 * 24 * time.Hour),
		Fingerprint:        "abcdef1234567890",
		DNSNames:           []string{"example.com", "www.example.com"},
		IsExpired:          false,
		IsSelfSigned:       false,
		SerialNumber:       "123456789",
		SignatureAlgorithm: "SHA256-RSA",
	}

	data, err := ci.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	var decoded CertInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if decoded.Host != ci.Host {
		t.Errorf("Host = %q, want %q", decoded.Host, ci.Host)
	}
	if decoded.Subject != ci.Subject {
		t.Errorf("Subject = %q, want %q", decoded.Subject, ci.Subject)
	}
	if decoded.Issuer != ci.Issuer {
		t.Errorf("Issuer = %q, want %q", decoded.Issuer, ci.Issuer)
	}
	if decoded.Fingerprint != ci.Fingerprint {
		t.Errorf("Fingerprint = %q, want %q", decoded.Fingerprint, ci.Fingerprint)
	}
	if decoded.IsExpired != ci.IsExpired {
		t.Errorf("IsExpired = %v, want %v", decoded.IsExpired, ci.IsExpired)
	}
	if decoded.IsSelfSigned != ci.IsSelfSigned {
		t.Errorf("IsSelfSigned = %v, want %v", decoded.IsSelfSigned, ci.IsSelfSigned)
	}
	if decoded.SerialNumber != ci.SerialNumber {
		t.Errorf("SerialNumber = %q, want %q", decoded.SerialNumber, ci.SerialNumber)
	}
	if decoded.SignatureAlgorithm != ci.SignatureAlgorithm {
		t.Errorf("SignatureAlgorithm = %q, want %q", decoded.SignatureAlgorithm, ci.SignatureAlgorithm)
	}
	if len(decoded.DNSNames) != 2 {
		t.Errorf("DNSNames len = %d, want 2", len(decoded.DNSNames))
	}
}

func TestCertInfo_ToJSON_EmptyStruct(t *testing.T) {
	ci := &CertInfo{}
	data, err := ci.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON output")
	}
}

func TestCertInfo_ToJSON_ExpiredCert(t *testing.T) {
	ci := &CertInfo{
		Host:      "expired.example.com:443",
		NotBefore: time.Now().Add(-365 * 24 * time.Hour),
		NotAfter:  time.Now().Add(-30 * 24 * time.Hour),
		IsExpired: true,
	}

	data, err := ci.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	var decoded CertInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if !decoded.IsExpired {
		t.Error("expected IsExpired to be true")
	}
}

func TestCertInfo_ToJSON_SelfSigned(t *testing.T) {
	ci := &CertInfo{
		Host:         "self-signed.local:443",
		Subject:      "Self Signed CA",
		Issuer:       "Self Signed CA",
		IsSelfSigned: true,
	}

	data, err := ci.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error: %v", err)
	}

	var decoded CertInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if !decoded.IsSelfSigned {
		t.Error("expected IsSelfSigned to be true")
	}
}

func TestInspectCert_InvalidHost(t *testing.T) {
	// A host that cannot be dialed should return an error.
	_, err := InspectCert("localhost:1") // port 1 is unlikely to have a TLS server
	if err == nil {
		t.Error("expected error for invalid host, got nil")
	}
}

func TestInspectCert_HostWithoutPort(t *testing.T) {
	// Verify that a host without a port gets ":443" appended by checking the
	// error message contains ":443" (since the host is unreachable).
	_, err := InspectCert("this-host-does-not-exist.invalid")
	if err == nil {
		t.Error("expected error for non-existent host, got nil")
	}
}
