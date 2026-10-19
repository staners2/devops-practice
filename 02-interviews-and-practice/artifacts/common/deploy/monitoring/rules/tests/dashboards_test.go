package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardMetricsQueries(t *testing.T) {
	tool := os.Getenv("VMALERT_TOOL")
	if tool == "" {
		t.Skip("set VMALERT_TOOL to validate dashboard MetricsQL with VictoriaMetrics")
	}
	if _, err := exec.LookPath(tool); err != nil {
		t.Fatal(err)
	}
	replace := strings.NewReplacer(
		"$__rate_interval", "5m", "$__range", "1h",
		"$namespace", "devops-practice", "$component", "api|worker",
		"$pod", "devops-practice-.*", "$node", "k3d-devops-practice-server-0",
		"${__from:date:seconds}", "946684800", "${__to:date:seconds}", "946685400",
	)
	var expressions []map[string]any
	for _, uid := range []string{"service-overview", "kubernetes-operations", "incident-drilldown", "postgresql-operations"} {
		content, err := os.ReadFile("../../dashboards/" + uid + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var dashboard struct {
			UID    string
			Panels []struct {
				Targets []struct {
					Expr       string
					Datasource struct{ UID string }
				}
			}
			Annotations struct {
				List []struct {
					Expr       string
					Datasource struct{ UID string }
				}
			}
		}
		if err := json.Unmarshal(content, &dashboard); err != nil {
			t.Fatal(err)
		}
		if dashboard.UID != uid {
			t.Fatalf("dashboard UID changed: got %q, want %q", dashboard.UID, uid)
		}
		add := func(expr, datasource string) {
			t.Helper()
			if datasource != "victoriametrics" && datasource != "loki" {
				t.Fatalf("unexpected datasource UID %q", datasource)
			}
			if datasource == "victoriametrics" {
				expressions = append(expressions, map[string]any{"record": fmt.Sprintf("dashboard_query_%d", len(expressions)), "expr": replace.Replace(expr)})
			}
		}
		for _, panel := range dashboard.Panels {
			for _, target := range panel.Targets {
				add(target.Expr, target.Datasource.UID)
			}
		}
		for _, annotation := range dashboard.Annotations.List {
			add(annotation.Expr, annotation.Datasource.UID)
		}
	}
	test := map[string]any{
		"groups": []any{map[string]any{"name": "dashboard-query-syntax", "rules": expressions}},
	}
	content, err := json.Marshal(test)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "dashboard_queries.yaml")
	if err := os.WriteFile(file, content, 0600); err != nil {
		t.Fatal(err)
	}
	validator := filepath.Join(filepath.Dir(tool), "vmalert-prod")
	output, err := exec.Command(validator, "-dryRun", "-rule="+file).CombinedOutput()
	if err != nil {
		t.Fatalf("dashboard MetricsQL validation: %v\n%s", err, output)
	}
	t.Logf("%d dashboard and annotation expressions accepted by VictoriaMetrics", len(expressions))
}
