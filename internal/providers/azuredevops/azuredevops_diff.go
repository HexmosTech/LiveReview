package azuredevops

import (
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"

	"github.com/livereview/internal/diffutil"
	"github.com/livereview/pkg/models"
	"github.com/pmezard/go-difflib/difflib"
)

// fetchBlob retrieves the raw text content of a blob by its object id (SHA-1).
// An empty/all-zero object id (used by Azure DevOps to represent "no content"
// for added/deleted files) returns an empty string without making a request.
func (p *Provider) fetchBlob(ctx context.Context, apiBase, project, repo, objectID string) (string, error) {
	if isEmptyObjectID(objectID) {
		return "", nil
	}

	// $format=octetstream is required to get raw blob bytes back; without it
	// (or relying on the Accept header, which p.applyAuth overwrites to
	// application/json anyway) the API returns a GitBlobRef JSON metadata
	// object ({objectId, size, url, _links}) instead of the file content -
	// confirmed against https://learn.microsoft.com/rest/api/azure/devops/git/blobs/get-blob.
	apiURL := fmt.Sprintf("%s/%s/_apis/git/repositories/%s/blobs/%s?api-version=%s&$format=octetstream",
		apiBase, neturl.PathEscape(project), neturl.PathEscape(repo), objectID, apiVersion)

	req, err := newRequest(ctx, http.MethodGet, apiURL)
	if err != nil {
		return "", err
	}
	p.applyAuth(req)

	resp, err := p.do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch blob %s: %w", objectID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read blob content: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("blob fetch failed (%d): %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}

// buildCodeDiff generates a models.CodeDiff for a single changed file by
// diffing its old/new blob content client-side (Azure DevOps has no
// server-side unified-diff endpoint for pull requests).
func buildCodeDiff(entry changeEntry, oldContent, newContent string) *models.CodeDiff {
	path := strings.TrimPrefix(entry.Item.Path, "/")
	oldPath := strings.TrimPrefix(entry.OriginalPath, "/")

	changeType := strings.ToLower(entry.ChangeType)
	isNew := strings.Contains(changeType, "add")
	isDeleted := strings.Contains(changeType, "delete")
	isRenamed := strings.Contains(changeType, "rename")

	fromFile := firstNonEmpty(oldPath, path)
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(oldContent),
		B:        difflib.SplitLines(newContent),
		FromFile: fromFile,
		ToFile:   path,
		Context:  3,
	}
	unified, _ := difflib.GetUnifiedDiffString(diff)
	hunks := diffutil.ParseUnifiedHunks(unified)

	return &models.CodeDiff{
		FilePath:    path,
		FileType:    getFileType(path),
		IsNew:       isNew,
		IsDeleted:   isDeleted,
		IsRenamed:   isRenamed,
		OldFilePath: oldPath,
		Hunks:       hunks,
	}
}

func getFileType(filename string) string {
	parts := strings.Split(filename, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return "unknown"
}
