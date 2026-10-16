package cmd

import (
	"context"
	"fmt"

	"google.golang.org/api/chat/v1"

	"github.com/openclaw/gogcli/internal/googleapi"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type ChatMessagesGetCmd struct {
	Message string `arg:"" name:"message" help:"Message resource (spaces/.../messages/...) or bare message ID"`
	Space   string `name:"space" help:"Space name (required when message is a bare ID)"`
	Markup  string `name:"markup" help:"Formatted text syntax: chat or markdown" enum:"chat,markdown," default:""`
}

func (c *ChatMessagesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	name, err := normalizeMessage(c.Space, c.Message)
	if err != nil {
		return usagef("invalid message: %v", err)
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}
	svc, err := chatService(ctx, account)
	if err != nil {
		return err
	}
	call := svc.Spaces.Messages.Get(name).Context(ctx)
	if markup := chatMarkupSyntax(c.Markup); markup != "" {
		call = call.MarkupSyntax(markup)
	}
	message, err := call.Do()
	if err != nil {
		return err
	}
	return writeChatMessage(ctx, message)
}

type ChatMessagesUpdateCmd struct {
	Message string  `arg:"" name:"message" help:"Message resource (spaces/.../messages/...) or bare message ID"`
	Space   string  `name:"space" help:"Space name (required when message is a bare ID)"`
	Text    *string `name:"text" help:"Replacement message text (required; empty explicitly clears text)"`
}

func (c *ChatMessagesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	name, err := normalizeMessage(c.Space, c.Message)
	if err != nil {
		return usagef("invalid message: %v", err)
	}
	if c.Text == nil {
		return usage("required: --text")
	}
	message := &chat.Message{Text: *c.Text, ForceSendFields: []string{"Text"}}
	if err = dryRunExit(ctx, flags, "chat.messages.update", map[string]any{
		"message": name, "updateMask": "text", "body": message,
	}); err != nil {
		return err
	}
	if googleapi.ReadOnly(ctx) || (flags != nil && flags.ReadOnly) {
		return googleapi.ErrReadOnly
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}
	svc, err := chatService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Spaces.Messages.Patch(name, message).UpdateMask("text").Context(ctx).Do()
	if err != nil {
		return err
	}
	return writeChatMessage(ctx, resp)
}

type ChatMessagesDeleteCmd struct {
	Message       string `arg:"" name:"message" help:"Message resource (spaces/.../messages/...) or bare message ID"`
	Space         string `name:"space" help:"Space name (required when message is a bare ID)"`
	DeleteReplies bool   `name:"delete-replies" help:"Also delete threaded replies (Google API force; distinct from CLI --force)"`
}

func (c *ChatMessagesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	name, err := normalizeMessage(c.Space, c.Message)
	if err != nil {
		return usagef("invalid message: %v", err)
	}
	if err = dryRunExit(ctx, flags, "chat.messages.delete", map[string]any{
		"message": name, "deleteReplies": c.DeleteReplies,
	}); err != nil {
		return err
	}
	if googleapi.ReadOnly(ctx) || (flags != nil && flags.ReadOnly) {
		return googleapi.ErrReadOnly
	}
	action := fmt.Sprintf("delete Chat message %s", name)
	if c.DeleteReplies {
		action += " and its threaded replies"
	}
	if err = confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), action); err != nil {
		return err
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if err = requireWorkspaceAccount(account); err != nil {
		return err
	}
	svc, err := chatService(ctx, account)
	if err != nil {
		return err
	}
	if _, err := svc.Spaces.Messages.Delete(name).Force(c.DeleteReplies).Context(ctx).Do(); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"deleted": name, "deleteReplies": c.DeleteReplies})
	}
	ui.FromContext(ctx).Out().Linef("deleted\t%s", name)
	return nil
}

func chatMarkupSyntax(markup string) string {
	switch markup {
	case "chat":
		return "MARKUP_SYNTAX_CHAT"
	case "markdown":
		return "MARKUP_SYNTAX_MARKDOWN"
	default:
		return ""
	}
}

func writeChatMessage(ctx context.Context, message *chat.Message) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(outfmt.WithUntrustedChatContent(ctx), stdoutWriter(ctx), map[string]any{"message": message})
	}
	return outfmt.WriteTable(ctx, stdoutWriter(ctx), compactChatRows([]*chat.Message{message}), chatMessageColumns())
}
