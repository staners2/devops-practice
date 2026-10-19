package notification

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const gitLabBodyLimit = 2 << 20

type GitLabSource struct {
	client      *http.Client
	baseURL     string
	projectPath string
	token       string
}

type gitLabProject struct {
	DefaultBranch string `json:"default_branch"`
}

type gitLabPipeline struct {
	ID        int64  `json:"id"`
	SHA       string `json:"sha"`
	Ref       string `json:"ref"`
	Source    string `json:"source"`
	WebURL    string `json:"web_url"`
	UpdatedAt string `json:"updated_at"`
}

type gitLabMergeRequest struct {
	IID    int64  `json:"iid"`
	Title  string `json:"title"`
	WebURL string `json:"web_url"`
}

type gitLabJob struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type gitLabDeployment struct {
	ID          int64  `json:"id"`
	SHA         string `json:"sha"`
	UpdatedAt   string `json:"updated_at"`
	Environment struct {
		Name        string `json:"name"`
		ExternalURL string `json:"external_url"`
	} `json:"environment"`
}

func NewGitLabSource(client *http.Client, cfg Config) *GitLabSource {
	return &GitLabSource{
		client:      client,
		baseURL:     strings.TrimRight(cfg.GitLabURL, "/"),
		projectPath: "/projects/" + url.PathEscape(cfg.GitLabProjectID),
		token:       cfg.GitLabReadToken,
	}
}

func (g *GitLabSource) Events(ctx context.Context) ([]Event, error) {
	var project gitLabProject
	if err := g.getJSON(ctx, g.projectPath, nil, &project); err != nil {
		return nil, err
	}

	pipelines, err := fetchGitLabPages[gitLabPipeline](ctx, g, g.projectPath+"/pipelines", url.Values{
		"status":   {"success"},
		"order_by": {"updated_at"},
		"sort":     {"asc"},
	})
	if err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(pipelines))
	for _, pipeline := range pipelines {
		occurredAt, err := time.Parse(time.RFC3339, pipeline.UpdatedAt)
		if err != nil {
			return nil, errors.New("GitLab pipeline has invalid updated_at")
		}
		switch {
		case pipeline.Source == "merge_request_event":
			mergeRequests, err := fetchGitLabPages[gitLabMergeRequest](ctx, g, fmt.Sprintf("%s/pipelines/%d/merge_requests", g.projectPath, pipeline.ID), nil)
			if err != nil {
				return nil, err
			}
			if len(mergeRequests) == 0 {
				continue
			}
			mergeRequest := mergeRequests[0]
			events = append(events, Event{
				Key:        fmt.Sprintf("pipeline:%d:mr", pipeline.ID),
				OccurredAt: occurredAt,
				Message: fmt.Sprintf(
					"MR !%d прошёл проверки и готов к ревью\nНазвание: %s\nMR: %s\nPipeline: %s",
					mergeRequest.IID,
					mergeRequest.Title,
					mergeRequest.WebURL,
					pipeline.WebURL,
				),
			})
		case pipeline.Ref == project.DefaultBranch:
			event, ok, err := g.readyImageEvent(ctx, pipeline, occurredAt)
			if err != nil {
				return nil, err
			}
			if ok {
				events = append(events, event)
			}
		}
	}

	deployments, err := fetchGitLabPages[gitLabDeployment](ctx, g, g.projectPath+"/deployments", url.Values{
		"status":   {"success"},
		"order_by": {"updated_at"},
		"sort":     {"asc"},
	})
	if err != nil {
		return nil, err
	}
	for _, deployment := range deployments {
		occurredAt, err := time.Parse(time.RFC3339, deployment.UpdatedAt)
		if err != nil {
			return nil, errors.New("GitLab deployment has invalid updated_at")
		}
		message := fmt.Sprintf(
			"Версия задеплоена\nEnvironment: %s\nCommit: %s",
			deployment.Environment.Name,
			deployment.SHA,
		)
		if deployment.Environment.ExternalURL != "" {
			message += "\nURL: " + deployment.Environment.ExternalURL
		}
		events = append(events, Event{
			Key:        fmt.Sprintf("deployment:%d", deployment.ID),
			OccurredAt: occurredAt,
			Message:    message,
		})
	}
	return events, nil
}

func (g *GitLabSource) readyImageEvent(ctx context.Context, pipeline gitLabPipeline, occurredAt time.Time) (Event, bool, error) {
	jobs, err := fetchGitLabPages[gitLabJob](ctx, g, fmt.Sprintf("%s/pipelines/%d/jobs", g.projectPath, pipeline.ID), nil)
	if err != nil {
		return Event{}, false, err
	}
	for _, job := range jobs {
		if job.Name != "publish-image" || job.Status != "success" {
			continue
		}
		body, status, err := g.get(ctx, fmt.Sprintf("%s/jobs/%d/artifacts/image.env", g.projectPath, job.ID), nil)
		if err != nil {
			return Event{}, false, err
		}
		if status == http.StatusNotFound {
			return Event{}, false, nil
		}
		if status < 200 || status >= 300 {
			return Event{}, false, fmt.Errorf("GitLab request failed with status %d", status)
		}
		metadata, ok := parseImageMetadata(body)
		if !ok {
			return Event{}, false, nil
		}
		return Event{
			Key:        fmt.Sprintf("pipeline:%d:ready", pipeline.ID),
			OccurredAt: occurredAt,
			Message: fmt.Sprintf(
				"Новая версия готова\nCommit: %s\nImage: %s\nDigest: %s\nPipeline: %s",
				pipeline.SHA,
				metadata["IMAGE_REFERENCE"],
				metadata["IMAGE_DIGEST"],
				pipeline.WebURL,
			),
		}, true, nil
	}
	return Event{}, false, nil
}

func parseImageMetadata(body []byte) (map[string]string, bool) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(body))
	for scanner.Scan() {
		name, value, found := strings.Cut(scanner.Text(), "=")
		if found && (name == "IMAGE_REFERENCE" || name == "IMAGE_DIGEST") {
			values[name] = strings.TrimSpace(value)
		}
	}
	return values, values["IMAGE_REFERENCE"] != "" && strings.HasPrefix(values["IMAGE_DIGEST"], "sha256:")
}

func fetchGitLabPages[T any](ctx context.Context, source *GitLabSource, path string, query url.Values) ([]T, error) {
	page := "1"
	var result []T
	for page != "" {
		requestQuery := cloneValues(query)
		requestQuery.Set("per_page", "100")
		requestQuery.Set("page", page)

		body, status, headers, err := source.getWithHeaders(ctx, path, requestQuery)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, fmt.Errorf("GitLab request failed with status %d", status)
		}
		var items []T
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, errors.New("decode GitLab response")
		}
		result = append(result, items...)
		page = strings.TrimSpace(headers.Get("X-Next-Page"))
	}
	return result, nil
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values)+2)
	for key, entries := range values {
		cloned[key] = append([]string(nil), entries...)
	}
	return cloned
}

func (g *GitLabSource) getJSON(ctx context.Context, path string, query url.Values, destination any) error {
	body, status, err := g.get(ctx, path, query)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("GitLab request failed with status %d", status)
	}
	if err := json.Unmarshal(body, destination); err != nil {
		return errors.New("decode GitLab response")
	}
	return nil
}

func (g *GitLabSource) get(ctx context.Context, path string, query url.Values) ([]byte, int, error) {
	body, status, _, err := g.getWithHeaders(ctx, path, query)
	return body, status, err
}

func (g *GitLabSource) getWithHeaders(ctx context.Context, path string, query url.Values) ([]byte, int, http.Header, error) {
	requestURL := g.baseURL + path
	if len(query) != 0 {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, 0, nil, errors.New("create GitLab request")
	}
	if g.token != "" {
		request.Header.Set("PRIVATE-TOKEN", g.token)
	}
	response, err := g.client.Do(request)
	if err != nil {
		return nil, 0, nil, errors.New("send GitLab request")
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, gitLabBodyLimit+1))
	if err != nil {
		return nil, 0, nil, errors.New("read GitLab response")
	}
	if len(body) > gitLabBodyLimit {
		return nil, 0, nil, errors.New("GitLab response exceeds 2 MiB")
	}
	return body, response.StatusCode, response.Header.Clone(), nil
}
