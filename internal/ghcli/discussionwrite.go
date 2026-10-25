package ghcli

import (
	"errors"
	"strings"
)

const addDiscussionCommentMutation = `mutation($discussion:ID!,$body:String!,$reply:ID){addDiscussionComment(input:{discussionId:$discussion,body:$body,replyToId:$reply}){comment{id}}}`

// AddDiscussionComment adds a comment to a discussion; with replyToID it is a reply in that top-level
// comment's thread. The body travels as JSON on stdin, the ids as variables.
func AddDiscussionComment(host, discussionID, body, replyToID string) error {
	if strings.TrimSpace(discussionID) == "" {
		return errors.New("no discussion to comment on")
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("an empty comment is not sent")
	}
	vars := map[string]any{"discussion": discussionID, "body": body}
	if replyToID != "" {
		vars["reply"] = replyToID
	}
	_, err := runWrite(host, addDiscussionCommentMutation, vars)
	return err
}
