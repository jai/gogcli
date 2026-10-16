package outfmt

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// WithUntrustedChatContent scopes conservative rich-content handling to full
// Chat output. Compact output retains its existing filtering and wrapping rules.
func WithUntrustedChatContent(ctx context.Context) context.Context {
	opts, ok := UntrustedWrapperFromContext(ctx)
	if !ok {
		return ctx
	}
	opts.chatContent = true

	return WithUntrustedWrapper(ctx, opts)
}

var (
	chatMessageResource    = regexp.MustCompile(`^spaces/[A-Za-z0-9_-]+/messages/[A-Za-z0-9_.-]+$`)
	chatSpaceResource      = regexp.MustCompile(`^spaces/[A-Za-z0-9_-]+$`)
	chatThreadResource     = regexp.MustCompile(`^spaces/[A-Za-z0-9_-]+/threads/[A-Za-z0-9_.-]+$`)
	chatAttachmentResource = regexp.MustCompile(`^spaces/[A-Za-z0-9_-]+/messages/[A-Za-z0-9_.-]+/attachments/[A-Za-z0-9_-]+$`)
	chatOpaqueID           = regexp.MustCompile(`^[A-Za-z0-9_./+=-]+$`)
)

func trustedChatString(path []string, value string) bool {
	// --select retains dotted paths; reconstruct them before classifying fields.
	parts := make([]string, 0, len(path))
	for _, part := range strings.Split(strings.Join(path, "."), ".") {
		if _, err := strconv.Atoi(part); err != nil {
			parts = append(parts, part)
		}
	}

	if len(parts) == 0 {
		return false
	}
	key := parts[len(parts)-1]

	parent := ""
	if len(parts) > 1 {
		parent = parts[len(parts)-2]
	}

	switch key {
	case "name":
		switch parent {
		case "", "message", "messages", "quotedMessageMetadata":
			return chatMessageResource.MatchString(value)
		case "space":
			return chatSpaceResource.MatchString(value)
		case "thread":
			return chatThreadResource.MatchString(value)
		case "sender", "user", "privateMessageViewer":
			return trustedChatUserResourcePattern.MatchString(value)
		case "customEmoji":
			return trustedChatCustomEmojiResourcePattern.MatchString(value)
		case "attachment":
			return chatAttachmentResource.MatchString(value)
		}
	case "url", "uri", "imageUrl", "iconUrl", "imageUri", "downloadUri", "thumbnailUri", "contentUrl", "originalUri", "redirectUri", "messageUrl", "temporaryImageUri", "avatarUrl", "startIconUri":
		return validChatURL(value)
	case "message":
		return parent == "chatSpaceLinkData" && chatMessageResource.MatchString(value)
	case "space":
		return parent == "chatSpaceLinkData" && chatSpaceResource.MatchString(value)
	case "thread":
		return parent == "chatSpaceLinkData" && chatThreadResource.MatchString(value)
	case "meetingCode":
		return parent == "meetSpaceLinkData" && chatOpaqueID.MatchString(value)
	case "eventId":
		return parent == "calendarEventLinkData" && chatOpaqueID.MatchString(value)
	case "threadKey":
		return parent == "thread" && chatOpaqueID.MatchString(value)
	case "nextPageToken", "clientAssignedMessageId":
		return chatOpaqueID.MatchString(value)
	case "attachmentUploadToken":
		return parent == "attachmentDataRef" && chatOpaqueID.MatchString(value)
	case "resourceName":
		return parent == "attachmentDataRef" && chatAttachmentResource.MatchString(value)
	case "driveFileId":
		return parent == "driveDataRef" && chatOpaqueID.MatchString(value)
	case "uid":
		return parent == "customEmoji" && chatOpaqueID.MatchString(value)
	case "createTime", "lastUpdateTime", "deleteTime":
		_, err := time.Parse(time.RFC3339Nano, value)
		return err == nil
	case "spaceMuteSetting":
		return value == "MUTED" || value == "UNMUTED" || value == "MUTE_SETTING_UNSPECIFIED"
	case "cardId":
		return parent == "cardsV2" && chatOpaqueID.MatchString(value)
	}
	// Card names, labels, titles, filenames, parameters and all other strings are
	// content, even when their key resembles generic output metadata (e.g. id).
	return false
}

func validChatURL(value string) bool {
	if strings.ContainsAny(value, "<>\"\\") || strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return false
	}
	parsed, err := url.ParseRequestURI(value)

	return err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Hostname() != "" && parsed.User == nil
}
