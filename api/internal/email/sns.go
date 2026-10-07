package email

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
)

type SNSMessage struct {
	Type             string
	MessageId        string
	Token            string
	TopicArn         string
	Subject          string
	Message          string
	Timestamp        string
	SignatureVersion string
	Signature        string
	SigningCertURL   string
	SubscribeURL     string
}

var snsHost = regexp.MustCompile(`^sns\.[a-z0-9-]+\.amazonaws\.com(\.cn)?$`)

// ValidSNSURL accepts only https URLs on an SNS endpoint, so a forged message cannot make the
// server fetch arbitrary URLs.
func ValidSNSURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	return snsHost.MatchString(strings.ToLower(u.Hostname()))
}

func ParseSNS(body []byte) (*SNSMessage, error) {
	var m SNSMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	if m.Type == "" || m.Signature == "" || m.SigningCertURL == "" || m.TopicArn == "" {
		return nil, errors.New("not an SNS message")
	}
	return &m, nil
}

func (m *SNSMessage) StringToSign() string {
	var b strings.Builder
	add := func(k, v string) { b.WriteString(k + "\n" + v + "\n") }
	add("Message", m.Message)
	add("MessageId", m.MessageId)
	switch m.Type {
	case "SubscriptionConfirmation", "UnsubscribeConfirmation":
		add("SubscribeURL", m.SubscribeURL)
		add("Timestamp", m.Timestamp)
		add("Token", m.Token)
	default:
		if m.Subject != "" {
			add("Subject", m.Subject)
		}
		add("Timestamp", m.Timestamp)
	}
	add("TopicArn", m.TopicArn)
	add("Type", m.Type)
	return b.String()
}

var ErrSNSSignature = errors.New("SNS signature is not valid")

func (m *SNSMessage) Verify(ctx context.Context, cert func(ctx context.Context, url string) (*x509.Certificate, error)) error {
	if !ValidSNSURL(m.SigningCertURL) {
		return ErrSNSSignature
	}
	c, err := cert(ctx, m.SigningCertURL)
	if err != nil {
		return err
	}
	pub, ok := c.PublicKey.(*rsa.PublicKey)
	if !ok {
		return ErrSNSSignature
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return ErrSNSSignature
	}
	data := []byte(m.StringToSign())
	switch m.SignatureVersion {
	case "1":
		sum := sha1.Sum(data)
		err = rsa.VerifyPKCS1v15(pub, crypto.SHA1, sum[:], sig)
	case "2":
		sum := sha256.Sum256(data)
		err = rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig)
	default:
		return ErrSNSSignature
	}
	if err != nil {
		return ErrSNSSignature
	}
	return nil
}

type SESRecipient struct {
	EmailAddress   string `json:"emailAddress"`
	Status         string `json:"status"`
	Action         string `json:"action"`
	DiagnosticCode string `json:"diagnosticCode"`
}

type SESNotification struct {
	NotificationType string `json:"notificationType"`
	EventType        string `json:"eventType"`
	Bounce           *struct {
		BounceType        string         `json:"bounceType"`
		BounceSubType     string         `json:"bounceSubType"`
		BouncedRecipients []SESRecipient `json:"bouncedRecipients"`
	} `json:"bounce"`
	Complaint *struct {
		ComplainedRecipients  []SESRecipient `json:"complainedRecipients"`
		ComplaintFeedbackType string         `json:"complaintFeedbackType"`
	} `json:"complaint"`
	Mail struct {
		MessageID     string `json:"messageId"`
		CommonHeaders struct {
			MessageID string `json:"messageId"`
		} `json:"commonHeaders"`
		Headers []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"headers"`
	} `json:"mail"`
}

func (n SESNotification) Kind() string {
	if n.NotificationType != "" {
		return n.NotificationType
	}
	return n.EventType
}

// OriginalMessageID is the Message-ID header we set, not the id SES assigned.
func (n SESNotification) OriginalMessageID() string {
	if id := NormalizeID(n.Mail.CommonHeaders.MessageID); id != "" {
		return id
	}
	for _, h := range n.Mail.Headers {
		if strings.EqualFold(h.Name, "Message-ID") {
			return NormalizeID(h.Value)
		}
	}
	return ""
}
