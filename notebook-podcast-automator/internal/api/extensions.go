package api

import (
	"context"
)

// ==========================================
// Expose Core Features Wrappers
// ==========================================

// Query 发送问题给 NotebookLM 并获取回答 (Chat)
// 这是一个便捷方法，封装了 GenerateFreeFormStreamed
func (c *Client) Query(ctx context.Context, projectID string, prompt string) (string, error) {
	// 使用 GenerateFreeFormStreamed 接口
	resp, err := c.GenerateFreeFormStreamed(projectID, prompt, nil)
	if err != nil {
		return "", err
	}
	// 返回完整回答
	return resp.Chunk, nil
}

// ListNotebooks 列出笔记本 (目前等同于最近查看)
func (c *Client) ListNotebooks(ctx context.Context) ([]*Notebook, error) {
	return c.ListRecentlyViewedProjects()
}
