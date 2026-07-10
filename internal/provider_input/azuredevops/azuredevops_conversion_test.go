package azuredevops

import "testing"

// realCommentEventPayload is a verbatim capture of a live
// ms.vss-code.git-pullrequest-comment-event notification pulled from a real
// Azure DevOps organization's subscription history. It is deliberately used
// as-is (not hand-simplified) because Microsoft's own published docs example
// for this event is wrong (it shows resource as {comment:{...},
// pullRequest:{...}}; the real payload has the comment fields directly on
// resource, with no "comment"/"pullRequest" wrapper and no repo/project
// names - only a repository GUID and a PR-number link). A previous
// implementation trusted the docs and silently dropped every comment event.
const realCommentEventPayload = `{
	"id": "31445d13-b9a7-4ec8-84be-49072a58cccc",
	"eventType": "ms.vss-code.git-pullrequest-comment-event",
	"publisherId": "tfs",
	"resource": {
		"id": 2,
		"parentCommentId": 1,
		"author": {
			"displayName": "linz07m",
			"id": "908a0455-ed58-4942-af05-e41517de65e7",
			"uniqueName": "linz07m@gmail.com",
			"imageUrl": "https://dev.azure.com/hexmos/_apis/GraphProfile/MemberAvatars/msa.MjM3MjhiYWQtMmM3Zi03MGUwLWE2ZjEtMDMzOTM5YWQ1Mzk1"
		},
		"content": "explain the issue\n",
		"publishedDate": "2026-07-04T15:52:07.413Z",
		"lastUpdatedDate": "2026-07-04T15:52:07.413Z",
		"lastContentUpdatedDate": "2026-07-04T15:52:07.413Z",
		"commentType": "text",
		"usersLiked": [],
		"_links": {
			"self": {
				"href": "https://dev.azure.com/hexmos/_apis/git/repositories/b6d60a77-7b16-449e-849d-26542944a3de/pullRequests/1/threads/5/comments/2"
			},
			"repository": {
				"href": "https://dev.azure.com/hexmos/e9dc8a5e-d8be-4091-b4fd-d9fd9bfa7d4d/_apis/git/repositories/b6d60a77-7b16-449e-849d-26542944a3de"
			},
			"threads": {
				"href": "https://dev.azure.com/hexmos/_apis/git/repositories/b6d60a77-7b16-449e-849d-26542944a3de/pullRequests/1/threads/5"
			},
			"pullRequests": {
				"href": "https://dev.azure.com/hexmos/_apis/git/pullRequests/1"
			}
		}
	},
	"resourceVersion": "1.0",
	"resourceContainers": {
		"collection": {
			"id": "e7518f2b-604a-4202-917d-17746a710eba",
			"baseUrl": "https://dev.azure.com/hexmos/"
		},
		"account": {
			"id": "123a2c8f-437d-4c96-8442-84170fdb877c",
			"baseUrl": "https://dev.azure.com/hexmos/"
		},
		"project": {
			"id": "e9dc8a5e-d8be-4091-b4fd-d9fd9bfa7d4d",
			"baseUrl": "https://dev.azure.com/hexmos/"
		}
	},
	"createdDate": "2026-07-04T15:52:13.8862869Z"
}`

func TestConvertAzureDevOpsCommentEvent_RealPayload(t *testing.T) {
	event, err := ConvertAzureDevOpsCommentEvent([]byte(realCommentEventPayload))
	if err != nil {
		t.Fatalf("ConvertAzureDevOpsCommentEvent() error = %v, want nil", err)
	}

	if event.Comment == nil {
		t.Fatal("event.Comment is nil, want populated")
	}
	if got, want := event.Comment.Body, "explain the issue\n"; got != want {
		t.Errorf("Comment.Body = %q, want %q", got, want)
	}
	if got, want := event.Comment.ID, "2"; got != want {
		t.Errorf("Comment.ID = %q, want %q", got, want)
	}
	if event.Comment.InReplyToID == nil || *event.Comment.InReplyToID != "1" {
		t.Errorf("Comment.InReplyToID = %v, want \"1\"", event.Comment.InReplyToID)
	}
	if got, want := event.Comment.Author.Username, "linz07m@gmail.com"; got != want {
		t.Errorf("Comment.Author.Username = %q, want %q", got, want)
	}

	threadID, ok := extractThreadIDFromMetadata(event.Comment.Metadata)
	if !ok || threadID != 5 {
		t.Errorf("thread_id metadata = (%v, %v), want (5, true)", threadID, ok)
	}

	if event.MergeRequest == nil {
		t.Fatal("event.MergeRequest is nil, want populated")
	}
	if event.MergeRequest.Number != 1 {
		t.Errorf("MergeRequest.Number = %d, want 1", event.MergeRequest.Number)
	}
	if got, want := event.MergeRequest.Metadata["repo_id"], "b6d60a77-7b16-449e-849d-26542944a3de"; got != want {
		t.Errorf("MergeRequest.Metadata[repo_id] = %v, want %v", got, want)
	}
	if got, want := event.MergeRequest.Metadata["org_url"], "https://dev.azure.com/hexmos"; got != want {
		t.Errorf("MergeRequest.Metadata[org_url] = %v, want %v (trailing slash must be trimmed)", got, want)
	}

	// Repository names are NOT present in this payload - FetchMergeRequestData
	// resolves them later via API using repo_id. FullName must stay empty here.
	if event.Repository.FullName != "" {
		t.Errorf("Repository.FullName = %q, want empty (names aren't in this payload)", event.Repository.FullName)
	}
	if got, want := event.Repository.ID, "b6d60a77-7b16-449e-849d-26542944a3de"; got != want {
		t.Errorf("Repository.ID = %q, want %q", got, want)
	}
}
