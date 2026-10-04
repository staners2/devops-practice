package notification

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gitLabConfig(serverURL string) Config {
	return Config{GitLabURL: serverURL, GitLabProjectID: "group/devops-practice"}
}

func writeJSON(t *testing.T, writer http.ResponseWriter, body string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write([]byte(body))
}

func TestGitLabEventsFollowPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/pipelines"):
			if request.URL.Query().Get("page") == "2" {
				writeJSON(t, writer, `[{"id":2,"sha":"bbbb","ref":"feature/two","source":"merge_request_event","web_url":"https://gitlab/p/2","updated_at":"2026-10-04T10:02:00Z"}]`)
				return
			}
			writer.Header().Set("X-Next-Page", "2")
			writeJSON(t, writer, `[{"id":1,"sha":"aaaa","ref":"feature/one","source":"merge_request_event","web_url":"https://gitlab/p/1","updated_at":"2026-10-04T10:01:00Z"}]`)
		case strings.HasSuffix(request.URL.Path, "/merge_requests"):
			id := strings.Split(request.URL.Path, "/")[5]
			writeJSON(t, writer, fmt.Sprintf(`[{"iid":%s,"title":"MR %s","web_url":"https://gitlab/mr/%s"}]`, id, id, id))
		case strings.HasSuffix(request.URL.Path, "/deployments"):
			writeJSON(t, writer, `[]`)
		default:
			writeJSON(t, writer, `{"default_branch":"main","web_url":"https://gitlab/group/devops-practice"}`)
		}
	}))
	defer server.Close()

	events, err := NewGitLabSource(server.Client(), gitLabConfig(server.URL)).Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Key != "pipeline:1:mr" || events[1].Key != "pipeline:2:mr" {
		t.Fatalf("events = %#v", events)
	}
}

func TestGitLabEventsBuildMergeRequestMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/pipelines"):
			writeJSON(t, writer, `[{"id":42,"sha":"abcdef","ref":"feature/ci","source":"merge_request_event","web_url":"https://gitlab.example/pipelines/42","updated_at":"2026-10-04T10:00:00Z"}]`)
		case strings.HasSuffix(request.URL.Path, "/merge_requests"):
			writeJSON(t, writer, `[{"iid":7,"title":"Настроить CI","web_url":"https://gitlab.example/merge_requests/7"}]`)
		case strings.HasSuffix(request.URL.Path, "/deployments"):
			writeJSON(t, writer, `[]`)
		default:
			writeJSON(t, writer, `{"default_branch":"main"}`)
		}
	}))
	defer server.Close()

	events, err := NewGitLabSource(server.Client(), gitLabConfig(server.URL)).Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := "MR !7 прошёл проверки и готов к ревью\nНазвание: Настроить CI\nMR: https://gitlab.example/merge_requests/7\nPipeline: https://gitlab.example/pipelines/42"
	if len(events) != 1 || events[0].Message != want {
		t.Fatalf("events = %#v, want message %q", events, want)
	}
}

func TestGitLabEventsBuildReadyImageMessageFromPublishArtifact(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/pipelines"):
			writeJSON(t, writer, `[{"id":43,"sha":"0123456789abcdef","ref":"main","source":"push","web_url":"https://gitlab.example/pipelines/43","updated_at":"2026-10-04T11:00:00Z"}]`)
		case strings.HasSuffix(request.URL.Path, "/jobs"):
			writeJSON(t, writer, `[{"id":99,"name":"publish-image","status":"success"}]`)
		case strings.HasSuffix(request.URL.Path, "/artifacts/image.env"):
			_, _ = writer.Write([]byte("IMAGE_REFERENCE=registry.gitlab.com/group/devops-practice:0123456789abcdef\nIMAGE_DIGEST=sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"))
		case strings.HasSuffix(request.URL.Path, "/deployments"):
			writeJSON(t, writer, `[]`)
		default:
			writeJSON(t, writer, `{"default_branch":"main"}`)
		}
	}))
	defer server.Close()

	events, err := NewGitLabSource(server.Client(), gitLabConfig(server.URL)).Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := "Новая версия готова\nCommit: 0123456789abcdef\nImage: registry.gitlab.com/group/devops-practice:0123456789abcdef\nDigest: sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\nPipeline: https://gitlab.example/pipelines/43"
	if len(events) != 1 || events[0].Key != "pipeline:43:ready" || events[0].Message != want {
		t.Fatalf("events = %#v", events)
	}
}

func TestGitLabEventsSkipReadyMessageUntilImageMetadataExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/pipelines"):
			writeJSON(t, writer, `[{"id":44,"sha":"abc","ref":"main","source":"push","web_url":"https://gitlab/p/44","updated_at":"2026-10-04T11:00:00Z"}]`)
		case strings.HasSuffix(request.URL.Path, "/jobs"):
			writeJSON(t, writer, `[{"id":100,"name":"publish-image","status":"success"}]`)
		case strings.HasSuffix(request.URL.Path, "/artifacts/image.env"):
			http.NotFound(writer, request)
		case strings.HasSuffix(request.URL.Path, "/deployments"):
			writeJSON(t, writer, `[]`)
		default:
			writeJSON(t, writer, `{"default_branch":"main"}`)
		}
	}))
	defer server.Close()

	events, err := NewGitLabSource(server.Client(), gitLabConfig(server.URL)).Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %#v", events)
	}
}

func TestGitLabEventsBuildDeploymentMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/pipelines"):
			writeJSON(t, writer, `[]`)
		case strings.HasSuffix(request.URL.Path, "/deployments"):
			writeJSON(t, writer, `[{"id":5,"sha":"01234567","updated_at":"2026-10-04T12:00:00Z","environment":{"name":"staging","external_url":"https://staging.example"}}]`)
		default:
			writeJSON(t, writer, `{"default_branch":"main"}`)
		}
	}))
	defer server.Close()

	events, err := NewGitLabSource(server.Client(), gitLabConfig(server.URL)).Events(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := "Версия задеплоена\nEnvironment: staging\nCommit: 01234567\nURL: https://staging.example"
	if len(events) != 1 || events[0].Key != "deployment:5" || events[0].Message != want {
		t.Fatalf("events = %#v", events)
	}
}

func TestGitLabEventsSendPrivateTokenHeaderOnlyWhenConfigured(t *testing.T) {
	for _, test := range []struct {
		name  string
		token string
		want  string
	}{
		{name: "public"},
		{name: "private", token: "read-secret", want: "read-secret"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if got := request.Header.Get("PRIVATE-TOKEN"); got != test.want {
					t.Fatalf("PRIVATE-TOKEN = %q, want %q", got, test.want)
				}
				if strings.HasSuffix(request.URL.Path, "/pipelines") || strings.HasSuffix(request.URL.Path, "/deployments") {
					writeJSON(t, writer, `[]`)
					return
				}
				writeJSON(t, writer, `{"default_branch":"main"}`)
			}))
			defer server.Close()

			cfg := gitLabConfig(server.URL)
			cfg.GitLabReadToken = test.token
			if _, err := NewGitLabSource(server.Client(), cfg).Events(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
