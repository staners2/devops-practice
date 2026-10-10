package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAlertmanagerRoutingAndInhibition(t *testing.T) {
	amtool, alertmanager := os.Getenv("AMTOOL"), os.Getenv("ALERTMANAGER")
	if amtool == "" || alertmanager == "" {
		t.Skip("set AMTOOL and ALERTMANAGER to run the Alertmanager integration test")
	}
	for _, binary := range []string{amtool, alertmanager} {
		if _, err := exec.LookPath(binary); err != nil {
			t.Fatal(err)
		}
	}

	temp := t.TempDir()
	config, err := os.ReadFile("../../alertmanager/alertmanager.yaml")
	if err != nil {
		t.Fatal(err)
	}
	template, err := filepath.Abs("../../alertmanager/telegram.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"bot_token": "test-token", "chat_id": "-100123456"} {
		if err := os.WriteFile(filepath.Join(temp, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	configText := strings.NewReplacer(
		"/etc/alertmanager/telegram/bot_token", filepath.Join(temp, "bot_token"),
		"/etc/alertmanager/telegram/chat_id", filepath.Join(temp, "chat_id"),
		"/etc/alertmanager/config/telegram.tmpl", template,
	).Replace(string(config))
	configPath := filepath.Join(temp, "alertmanager.yaml")
	if err := os.WriteFile(configPath, []byte(configText), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		output, err := exec.Command(amtool, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("amtool %v: %v\n%s", args, err, output)
		}
		return string(output)
	}
	run("check-config", "--enable-feature=utf8-strict-mode", configPath)
	for _, severity := range []string{"critical", "warning"} {
		run("config", "routes", "test", "--enable-feature=utf8-strict-mode",
			"--config.file="+configPath, "--verify.receivers=telegram",
			"alertname=APIHighErrorRate", "namespace=devops-practice",
			"service=devops-practice", "severity="+severity)
	}
	t.Log("canonical config passed UTF-8 strict validation; critical and warning route to telegram")

	for _, status := range []string{"firing", "resolved"} {
		data := map[string]any{
			"status":      status,
			"groupLabels": map[string]string{"alertname": "APIHighErrorRate", "namespace": "devops-practice", "service": "devops-practice"},
			"alerts": []map[string]any{{
				"status": status,
				"labels": map[string]string{"severity": "warning"},
				"annotations": map[string]string{
					"summary": "Высокая доля ошибок API", "description": "Доля 5xx превышает 5%.",
					"dashboard_url": "http://localhost:3000/d/service-overview",
					"runbook_url":   "docs/runbooks/observability.md#apihigherrorrate",
				},
				"startsAt": "2026-10-05T00:00:00Z", "endsAt": "2026-10-05T00:05:00Z",
			}},
		}
		content, err := json.Marshal(data)
		if err != nil {
			t.Fatal(err)
		}
		dataPath := filepath.Join(temp, status+".json")
		if err := os.WriteFile(dataPath, content, 0600); err != nil {
			t.Fatal(err)
		}
		message := run("template", "render", "--template.glob="+template,
			"--template.text={{ template \"devops-practice.telegram\" . }}", "--template.data="+dataPath)
		title := "СРАБОТАЛО"
		if status == "resolved" {
			title = "ВОССТАНОВЛЕНО"
		}
		for _, expected := range []string{title, "Высокая доля ошибок API", "devops-practice", "http://localhost:3000/d/service-overview", "docs/runbooks/observability.md#apihigherrorrate"} {
			if !strings.Contains(message, expected) {
				t.Fatalf("%s message missing %q: %s", status, expected, message)
			}
		}
	}
	t.Log("firing and resolved Telegram templates render in Russian")

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	receiverStart, receiverEnd := strings.Index(configText, "\n    telegram_configs:"), strings.Index(configText, "\ntemplates:")
	if receiverStart < 0 || receiverEnd < receiverStart {
		t.Fatal("canonical receiver layout changed")
	}
	// Only the transport changes: production route timing, grouping and inhibition stay intact.
	localConfig := configText[:receiverStart] + "\n    webhook_configs:\n      - url: " + receiver.URL + "\n        send_resolved: true\n" + configText[receiverEnd:]
	if err := os.WriteFile(configPath, []byte(localConfig), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	process := exec.Command(alertmanager, "--config.file="+configPath, "--storage.path="+filepath.Join(temp, "data"),
		"--web.listen-address="+address, "--cluster.listen-address=", "--enable-feature=utf8-strict-mode", "--log.level=error")
	var logs bytes.Buffer
	process.Stdout, process.Stderr = &logs, &logs
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := process.Process.Kill(); err != nil && !strings.Contains(err.Error(), "process already finished") {
			t.Error(err)
		}
		_ = process.Wait()
		if t.Failed() {
			t.Log(logs.String())
		}
	}()
	client := &http.Client{Timeout: 2 * time.Second}
	baseURL := "http://" + address
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, err := client.Get(baseURL + "/-/ready")
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("local Alertmanager did not become ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	start := time.Now().UTC().Add(-time.Minute)
	makeAlert := func(name, severity, service, namespace, component string) map[string]any {
		return map[string]any{
			"labels":      map[string]string{"alertname": name, "severity": severity, "service": service, "namespace": namespace, "component": component},
			"annotations": map[string]string{"summary": name},
			"startsAt":    start, "endsAt": start.Add(time.Hour),
		}
	}
	critical := makeAlert("DependencyUnavailable", "critical", "devops-practice", "devops-practice", "api")
	warnings := []map[string]any{
		makeAlert("APIHighErrorRate", "warning", "devops-practice", "devops-practice", "api"),
		makeAlert("APIHighErrorRate", "warning", "devops-practice", "devops-practice", "worker"),
		makeAlert("APIHighErrorRate", "warning", "other-service", "devops-practice", "api"),
		makeAlert("APIHighErrorRate", "warning", "devops-practice", "other-namespace", "api"),
	}
	post := func(alerts []map[string]any) {
		t.Helper()
		content, err := json.Marshal(alerts)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Post(baseURL+"/api/v2/alerts", "application/json", bytes.NewReader(content))
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("post alerts: HTTP %d", response.StatusCode)
		}
	}
	post(append([]map[string]any{critical}, warnings...))
	type group struct {
		Labels map[string]string
		Alerts []struct {
			Labels map[string]string
			Status struct{ InhibitedBy []string }
		}
	}
	check := func(expectInhibited bool) error {
		response, err := client.Get(baseURL + "/api/v2/alerts/groups")
		if err != nil {
			return err
		}
		defer response.Body.Close()
		var groups []group
		if err := json.NewDecoder(response.Body).Decode(&groups); err != nil {
			return err
		}
		checked, grouped := 0, false
		for _, g := range groups {
			if g.Labels["alertname"] != "APIHighErrorRate" {
				continue
			}
			sameService := g.Labels["service"] == "devops-practice" && g.Labels["namespace"] == "devops-practice"
			if sameService {
				if len(g.Alerts) != 2 {
					return fmt.Errorf("same alertname/namespace/service must group two components: got %d", len(g.Alerts))
				}
				grouped = true
			}
			for _, a := range g.Alerts {
				if (len(a.Status.InhibitedBy) > 0) != (sameService && expectInhibited) {
					return fmt.Errorf("unexpected inhibition for %v: %v", a.Labels, a.Status.InhibitedBy)
				}
				checked++
			}
		}
		if checked != 4 || !grouped {
			return fmt.Errorf("expected four warning alerts and one combined group, got %d", checked)
		}
		return nil
	}
	for _, inhibited := range []bool{true, false} {
		if !inhibited {
			critical["endsAt"] = time.Now().UTC().Add(-time.Second)
			post([]map[string]any{critical})
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			err := check(inhibited)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal(err)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	t.Log("real Alertmanager grouped components; critical inhibited only matching service/namespace; resolving critical removed inhibition")
}
