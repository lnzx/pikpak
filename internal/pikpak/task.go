package pikpak

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Client) CreateOfflineTask(ctx context.Context, rawURL, parentID string) (*OfflineTask, error) {
	body := map[string]any{
		"kind":        fileKindFile,
		"upload_type": "UPLOAD_TYPE_URL",
		"url": map[string]string{
			"url": rawURL,
		},
	}
	if parentID != "" {
		body["parent_id"] = parentID
	}
	var resp offlineDownloadResp
	if err := c.doJSON(ctx, http.MethodPost, apiDrive+"/drive/v1/files", body, &resp); err != nil {
		return nil, err
	}
	return &resp.Task, nil
}

func (c *Client) OfflineTasks(ctx context.Context) ([]OfflineTask, error) {
	values := url.Values{}
	values.Set("type", "offline")
	values.Set("thumbnail_size", "SIZE_SMALL")
	values.Set("limit", "10000")
	values.Set("with", "reference_resource")
	//values.Set("filters", `{"phase":{"in":"PHASE_TYPE_RUNNING,PHASE_TYPE_ERROR,PHASE_TYPE_COMPLETE,PHASE_TYPE_PENDING"}}`)
	var out []OfflineTask
	for {
		var resp offlineListResp
		u := apiDrive + "/drive/v1/tasks?" + values.Encode()
		if err := c.doJSON(ctx, http.MethodGet, u, nil, &resp); err != nil {
			return nil, err
		}
		out = append(out, resp.Tasks...)
		if resp.NextPageToken == "" {
			break
		}
		values.Set("page_token", resp.NextPageToken)
	}
	return out, nil
}

func (c *Client) ClearTasks(ctx context.Context, deleteFiles bool) error {
	body := map[string]any{
		"type":         "offline",
		"delete_files": deleteFiles,
		"phases":       []string{"PHASE_TYPE_COMPLETE", "PHASE_TYPE_ERROR", "PHASE_TYPE_RUNNING", "PHASE_TYPE_PENDING"},
	}
	u := apiDrive + "/drive/v1/tasks:clear"
	return c.doJSON(ctx, http.MethodPost, u, body, nil)
}

func (c *Client) DeleteTasks(ctx context.Context, deleteFiles bool, ids []string) error {
	values := url.Values{}
	values.Set("delete_files", strconv.FormatBool(deleteFiles))
	for _, id := range ids {
		values.Add("task_ids", id)
	}
	u := apiDrive + "/drive/v1/tasks?" + values.Encode()
	return c.doJSON(ctx, http.MethodDelete, u, nil, nil)
}
