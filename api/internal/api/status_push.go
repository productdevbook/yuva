package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/productdevbook/yuva/api/internal/oas"
)

const maxStatusPushBytes = 1 << 20

var (
	errPushTooLarge   = problem(http.StatusRequestEntityTooLarge, "too_large", "the body is larger than 1 MiB")
	errPushUnreadable = errValidation(`the body is none of {"state","message"}, an Alertmanager webhook or a Grafana alert webhook`)
)

func readLimited(r *http.Request, n int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, n+1))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || int64(len(body)) > n {
		return nil, errPushTooLarge
	}
	if err != nil {
		return nil, errPushUnreadable
	}
	return body, nil
}

type pushAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type pushEnvelope struct {
	State             *string           `json:"state"`
	Message           string            `json:"message"`
	DurationMs        json.RawMessage   `json:"duration_ms"`
	Status            string            `json:"status"`
	Alerts            []pushAlert       `json:"alerts"`
	OrgID             json.RawMessage   `json:"orgId"`
	Title             string            `json:"title"`
	RuleName          string            `json:"ruleName"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	CommonLabels      map[string]string `json:"commonLabels"`
}

// parseStatusPush reads a pushed state: Yuva's own {"state","message"}, an Alertmanager webhook or
// a Grafana alert webhook (Grafana Alerting or legacy). take is false for a body that reports no
// change, such as Grafana's no_data.
func parseStatusPush(body []byte) (monitorResult, bool, error) {
	var e pushEnvelope
	if err := json.Unmarshal(body, &e); err != nil {
		return monitorResult{}, false, errPushUnreadable
	}
	switch {
	case e.Alerts != nil && (e.Status == "firing" || e.Status == "resolved"):
		source := string(oas.StatusCheckAlertmanager)
		if len(e.OrgID) > 0 {
			source = string(oas.StatusCheckGrafana)
		}
		r := monitorResult{ok: e.Status == "resolved", source: source, message: alertMessage(e)}
		if !r.ok {
			r.errText = "alert firing"
		}
		return r, true, nil
	case e.State != nil:
		switch st := *e.State; st {
		case stOperational, stDegraded, stPartialOutage, stMajorOutage:
			d, err := pushDuration(e.DurationMs)
			if err != nil {
				return monitorResult{}, false, err
			}
			if st == stOperational {
				return monitorResult{ok: true, source: string(oas.StatusCheckPush), message: e.Message, durationMs: d}, true, nil
			}
			return monitorResult{source: string(oas.StatusCheckPush), state: st, message: e.Message, errText: st, durationMs: d}, true, nil
		case string(oas.GrafanaAlerting):
			return monitorResult{source: string(oas.StatusCheckGrafana), message: legacyGrafanaMessage(e), errText: "alert firing"}, true, nil
		case string(oas.GrafanaOk):
			return monitorResult{ok: true, source: string(oas.StatusCheckGrafana), message: legacyGrafanaMessage(e)}, true, nil
		case string(oas.GrafanaNoData), string(oas.GrafanaPaused), string(oas.GrafanaPending):
			return monitorResult{}, false, nil
		}
	}
	return monitorResult{}, false, errPushUnreadable
}

const maxPushDurationMs = 120_000

func pushDuration(raw json.RawMessage) (*int32, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var v int32
	if err := json.Unmarshal(raw, &v); err != nil || v < 0 || v > maxPushDurationMs {
		return nil, errPushUnreadable
	}
	return &v, nil
}

func alertMessage(e pushEnvelope) string {
	for _, k := range []string{"summary", "description"} {
		if v := strings.TrimSpace(e.CommonAnnotations[k]); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(e.Title); v != "" {
		return v
	}
	for _, a := range e.Alerts {
		if a.Status != e.Status {
			continue
		}
		for _, k := range []string{"summary", "description"} {
			if v := strings.TrimSpace(a.Annotations[k]); v != "" {
				return v
			}
		}
		if v := a.Labels["alertname"]; v != "" {
			return v
		}
	}
	return e.CommonLabels["alertname"]
}

func legacyGrafanaMessage(e pushEnvelope) string {
	for _, v := range []string{e.Title, e.RuleName, e.Message} {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}
