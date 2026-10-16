package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/chat/v1"
	gapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleapi"
)

const richChatFixture = `{
 "name":"spaces/AAA/messages/m1", "text":"hostile text", "formattedText":"**hostile text**",
 "sender":{"name":"users/123","displayName":"hostile sender"},
 "thread":{"name":"spaces/AAA/threads/t1","threadKey":"hostile-thread-1"}, "clientAssignedMessageId":"client-hostile", "space":{"name":"spaces/AAA"},
 "annotations":[{"type":"RICH_LINK","richLinkMetadata":{"uri":"https://example.com/doc","driveLinkData":{"driveDataRef":{"driveFileId":"file-123"}}}}],
 "attachment":[{"name":"spaces/AAA/messages/m1/attachments/a1","contentName":"hostile.png","downloadUri":"https://example.com/file","thumbnailUri":"https://example.com/thumb","attachmentDataRef":{"resourceName":"spaces/AAA/messages/m1/attachments/a1","attachmentUploadToken":"upload-token"}}],
 "cards":[{"header":{"title":"hostile v1","subtitle":"hostile subtitle","imageUrl":"https://example.com/v1.png"},"sections":[{"widgets":[{"textParagraph":{"text":"hostile paragraph"}}]}]}],
 "cardsV2":[{"cardId":"hostile-card-1","card":{"header":{"title":"hostile v2","subtitle":"hostile subtitle","imageUrl":"https://example.com/v2.png","imageAltText":"hostile alt"},"sections":[{"widgets":[{"decoratedText":{"topLabel":"hostile top","text":"hostile decorated","bottomLabel":"hostile bottom"}},{"buttonList":{"buttons":[{"text":"hostile button","altText":"hostile button alt","onClick":{"openLink":{"url":"https://example.com/button"}}}]}},{"image":{"imageUrl":"https://example.com/image.png","altText":"hostile image"}}]}]}}],
 "attachedGifs":[{"uri":"https://example.com/animated.gif"}],
 "accessoryWidgets":[{"buttonList":{"buttons":[{"text":"hostile accessory"}]}}],
 "emojiReactionSummaries":[{"emoji":{"customEmoji":{"name":"customEmojis/abc","emojiName":":hostile:","uid":"abc","temporaryImageUri":"https://example.com/emoji.png"}},"reactionCount":1}],
 "quotedMessageMetadata":{"name":"spaces/AAA/messages/quoted","forwardedMetadata":{"space":"spaces/FORWARDED"},"quotedMessageSnapshot":{"attachments":[{"name":"spaces/AAA/messages/quoted/attachments/qa1"}]}},
 "unknownFutureField":"not preserved by SDK"
}`

func TestChatMessageGetResourceAndMarkup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		markup string
	}{
		{"full", []string{"spaces/AAA/messages/m1"}, ""},
		{"bare", []string{"m1", "--space", "AAA", "--markup", "markdown"}, "MARKUP_SYNTAX_MARKDOWN"},
		{"prefixed", []string{"messages/m1", "--space", "spaces/AAA", "--markup", "chat"}, "MARKUP_SYNTAX_CHAT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/spaces/AAA/messages/m1" || r.URL.Query().Get("markupSyntax") != tc.markup {
					t.Errorf("request: %s %s", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, richChatFixture)
			})
			args := append([]string{"--account", "a@example.com", "--json", "chat", "messages", "get"}, tc.args...)
			result := executeWithChatTestService(t, args, svc)
			if result.err != nil {
				t.Fatal(result.err)
			}
			assertRichChatJSON(t, result.stdout)
			if !strings.Contains(result.stdout, `"message":`) {
				t.Fatal(result.stdout)
			}
		})
	}
}

func assertRichChatJSON(t *testing.T, output string) {
	t.Helper()
	for _, want := range []string{`"name": "spaces/AAA/messages/m1"`, `"formattedText"`, `"richLinkMetadata"`, `"attachment"`, `"cards"`, `"cardsV2"`, `"attachedGifs"`, `"accessoryWidgets"`, `https://example.com/emoji.png`, `hostile subtitle`, `hostile alt`} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q in %s", want, output)
		}
	}
	if strings.Contains(output, "unknownFutureField") || strings.Contains(output, `"resource":`) {
		t.Fatalf("not typed full output: %s", output)
	}
}

func TestChatMessageGetErrorsAndCancellation(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":{"code":404,"message":"missing"}}`, http.StatusNotFound)
		})
		result := executeWithChatTestService(t, []string{"--account", "a@example.com", "chat", "messages", "get", "spaces/AAA/messages/missing"}, svc)
		var apiErr *gapi.Error
		if !errors.As(result.err, &apiErr) || apiErr.Code != 404 {
			t.Fatalf("error: %v", result.err)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) { t.Error("cancelled request reached server") })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		ctx = app.WithRuntime(ctx, &app.Runtime{Services: app.Services{Chat: fixedGoogleTestService(svc)}})
		err := (&ChatMessagesGetCmd{Message: "spaces/AAA/messages/m1"}).Run(ctx, &RootFlags{Account: "a@example.com"})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error: %v", err)
		}
	})
}

func TestChatFullValidationBeforeService(t *testing.T) {
	for _, args := range [][]string{
		{"list", "spaces/AAA", "--full"},
		{"search", "query", "--full"},
		{"get", "m1"},
		{"get", "spaces/AAA/messages/"},
		{"update", "spaces/AAA/messages/m1"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			result := executeWithChatTestServiceFactory(t, append([]string{"--account", "user@gmail.com", "chat", "messages"}, args...), unexpectedChatTestService(t, "validation must precede service"))
			if ExitCode(result.err) != 2 || strings.Contains(result.err.Error(), "Workspace") {
				t.Fatalf("error: %v", result.err)
			}
		})
	}
}

func TestChatFullListPaginationAndMarkup(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{false: "page", true: "all"}[all], func(t *testing.T) {
			calls := 0
			svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/v1/spaces/AAA/messages" || r.URL.Query().Get("markupSyntax") != "MARKUP_SYNTAX_MARKDOWN" || r.URL.Query().Get("pageSize") != "1" {
					t.Errorf("request: %s %s", r.Method, r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Query().Get("pageToken") {
				case "start":
					io.WriteString(w, `{"messages":[`+richChatFixture+`],"nextPageToken":"next"}`)
				case "next":
					io.WriteString(w, `{"messages":[{"name":"spaces/AAA/messages/card-only","cardsV2":[{"card":{"header":{"title":"card only"}}}]}]}`)
				default:
					t.Errorf("page token: %s", r.URL)
				}
			})
			args := []string{"--account", "a@example.com", "--json", "chat", "messages", "list", "AAA", "--full", "--page", "start", "--max", "1", "--markup", "markdown"}
			if all {
				args = append(args, "--all")
			}
			result := executeWithChatTestService(t, args, svc)
			if result.err != nil {
				t.Fatal(result.err)
			}
			assertRichChatJSON(t, result.stdout)
			var output struct {
				Messages      []*chat.Message
				NextPageToken string
			}
			if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
				t.Fatal(err)
			}
			want := 1
			token := "next"
			if all {
				want = 2
				token = ""
			}
			if len(output.Messages) != want || calls != want || output.NextPageToken != token {
				t.Fatalf("calls=%d output=%s", calls, result.stdout)
			}
		})
	}
}

func TestChatFullSearchMetadataAndPagination(t *testing.T) {
	calls := 0
	svc := newChatSearchTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req chat.SearchMessagesRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.View != "SEARCH_MESSAGES_VIEW_FULL" || req.MarkupSyntax != "MARKUP_SYNTAX_CHAT" {
			t.Errorf("request: %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		if req.PageToken == "" {
			io.WriteString(w, `{"results":[{"message":`+richChatFixture+`,"read":false,"spaceMuteSetting":"MUTED"}],"nextPageToken":"next"}`)
			return
		}
		if req.PageToken != "next" {
			t.Errorf("page token %q", req.PageToken)
		}
		io.WriteString(w, `{"results":[{"message":{"name":"spaces/AAA/messages/card-only","cards":[{"header":{"title":"card only"}}]}},null,{}]}`)
	})
	result := executeWithChatSearchTestService(t, []string{"--account", "a@example.com", "--json", "chat", "messages", "search", "query", "--full", "--view", "full", "--markup", "chat", "--all"}, svc)
	if result.err != nil {
		t.Fatal(result.err)
	}
	assertRichChatJSON(t, result.stdout)
	var output struct {
		Results       []*googleapi.ChatSearchResult
		NextPageToken string
	}
	if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(output.Results) != 2 || output.Results[0].Read == nil || *output.Results[0].Read || output.Results[0].SpaceMuteSetting != "MUTED" || output.Results[1].Read != nil || output.NextPageToken != "" {
		t.Fatalf("metadata lost: %s", result.stdout)
	}
}

func TestChatFullEmpty(t *testing.T) {
	for _, response := range []string{`{}`, `{"messages":[null],"results":[null,{}]}`} {
		t.Run(response, func(t *testing.T) {
			for _, command := range []string{"list", "search"} {
				t.Run(command, func(t *testing.T) {
					handler := func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						io.WriteString(w, response)
					}
					args := []string{"--account", "a@example.com", "--json", "chat", "messages", command, "AAA", "--full", "--fail-empty"}
					var result executeTestResult
					if command == "list" {
						result = executeWithChatTestService(t, args, useFakeChatService(t, handler))
					} else {
						result = executeWithChatSearchTestService(t, args, newChatSearchTestClient(t, handler))
					}
					if ExitCode(result.err) != 3 || !json.Valid([]byte(result.stdout)) {
						t.Fatalf("result: %+v", result)
					}
				})
			}
		})
	}
}

func TestChatMessageUpdatePatch(t *testing.T) {
	for _, text := range []string{"replacement", "", "  preserve whitespace\n"} {
		t.Run(text, func(t *testing.T) {
			svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch || r.URL.Path != "/v1/spaces/AAA/messages/m1" || r.URL.Query().Get("updateMask") != "text" || r.URL.Query().Has("allowMissing") {
					t.Errorf("request: %s %s", r.Method, r.URL)
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if len(body) != 1 || body["text"] != text {
					t.Errorf("body: %#v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, richChatFixture)
			})
			result := executeWithChatTestService(t, []string{"--account", "a@example.com", "--json", "chat", "messages", "edit", "m1", "--space", "AAA", "--text=" + text}, svc)
			if result.err != nil {
				t.Fatal(result.err)
			}
			assertRichChatJSON(t, result.stdout)
		})
	}
}

func TestChatMessageDeleteForceSeparation(t *testing.T) {
	for _, alias := range []string{"delete", "rm", "remove", "del"} {
		t.Run(alias, func(t *testing.T) {
			for _, replies := range []bool{false, true} {
				svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
					want := "false"
					if replies {
						want = "true"
					}
					if r.Method != http.MethodDelete || r.URL.Path != "/v1/spaces/AAA/messages/m1" || r.URL.Query().Get("force") != want {
						t.Errorf("request: %s %s", r.Method, r.URL)
					}
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{}`)
				})
				args := []string{"--account", "a@example.com", "--json", "--no-input", "--force", "chat", "messages", alias, "m1", "--space", "AAA"}
				if replies {
					args = append(args, "--delete-replies")
				}
				result := executeWithChatTestService(t, args, svc)
				if result.err != nil {
					t.Fatal(result.err)
				}
				var receipt struct {
					Deleted       string
					DeleteReplies bool
				}
				json.Unmarshal([]byte(result.stdout), &receipt)
				if receipt.Deleted != "spaces/AAA/messages/m1" || receipt.DeleteReplies != replies {
					t.Fatal(result.stdout)
				}
			}
		})
	}
}

func TestChatMessageMutationGuards(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
		code int
	}{
		{"missing text", []string{"update", "spaces/AAA/messages/m1"}, "required: --text", 2},
		{"confirmation", []string{"delete", "spaces/AAA/messages/m1", "--no-input"}, "without --force", 2},
		{"reply confirmation", []string{"delete", "spaces/AAA/messages/m1", "--delete-replies", "--no-input"}, "and its threaded replies", 2},
		{"readonly update", []string{"update", "spaces/AAA/messages/m1", "--text=x", "--readonly"}, "--readonly", 2},
		{"readonly delete", []string{"delete", "spaces/AAA/messages/m1", "--force", "--readonly"}, "--readonly", 2},
		{"dry update", []string{"update", "m1", "--space", "AAA", "--text=", "--dry-run"}, "", 0},
		{"dry delete", []string{"delete", "m1", "--space", "AAA", "--delete-replies", "--no-input", "--dry-run"}, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := executeWithChatTestServiceFactory(t, append([]string{"--json", "chat", "messages"}, tc.args...), unexpectedChatTestService(t, "must not create service"))
			if ExitCode(result.err) != tc.code {
				t.Fatalf("error: %v", result.err)
			}
			if tc.want != "" && (result.err == nil || !strings.Contains(result.err.Error(), tc.want)) {
				t.Fatalf("error: %v", result.err)
			}
			if tc.code == 0 && (!strings.Contains(result.stdout, "spaces/AAA/messages/m1") || !strings.Contains(result.stdout, "dry_run")) {
				t.Fatal(result.stdout)
			}
		})
	}
}

func TestChatMessageMutationProviderFailures(t *testing.T) {
	for _, command := range []string{"update", "delete"} {
		t.Run(command, func(t *testing.T) {
			svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":{"code":403,"message":"permission denied"}}`, http.StatusForbidden)
			})
			args := []string{"--account", "a@example.com", "--json", "--force", "chat", "messages", command, "spaces/AAA/messages/m1"}
			if command == "update" {
				args = append(args, "--text=x")
			}
			result := executeWithChatTestService(t, args, svc)
			var apiErr *gapi.Error
			if !errors.As(result.err, &apiErr) || apiErr.Code != 403 || strings.Contains(result.stdout, `"deleted"`) || strings.Contains(result.stdout, `"message": {`) {
				t.Fatalf("result: %+v", result)
			}
		})
	}
}

func TestChatFullWrappingAndProjection(t *testing.T) {
	for _, command := range []string{"get", "list", "search"} {
		for _, wrap := range []bool{false, true} {
			for _, projection := range []bool{false, true} {
				t.Run(command+map[bool]string{false: "/raw", true: "/wrapped"}[wrap]+map[bool]string{false: "", true: "/selected"}[projection], func(t *testing.T) {
					args := []string{"--account", "a@example.com", "--json", "chat", "messages", command}
					switch command {
					case "get":
						args = append(args, "spaces/AAA/messages/m1")
					case "list":
						args = append(args, "AAA", "--full")
					case "search":
						args = append(args, "query", "--full")
					}
					if wrap {
						args = append(args, "--wrap-untrusted")
					}
					if projection {
						prefix := ""
						if command == "search" {
							prefix = "message."
						}
						fields := []string{"name", "thread.threadKey", "clientAssignedMessageId", "cardsV2.0.cardId", "quotedMessageMetadata.forwardedMetadata.space", "quotedMessageMetadata.quotedMessageSnapshot.attachments.0.name", "sender.name", "sender.displayName", "attachment.0.contentName", "cardsV2.0.card.header.title", "cardsV2.0.card.header.subtitle", "cardsV2.0.card.header.imageAltText", "cardsV2.0.card.header.imageUrl", "cardsV2.0.card.sections.0.widgets.0.decoratedText.topLabel", "cardsV2.0.card.sections.0.widgets.1.buttonList.buttons.0.altText", "cardsV2.0.card.sections.0.widgets.1.buttonList.buttons.0.onClick.openLink.url"}
						for i := range fields {
							fields[i] = prefix + fields[i]
						}
						args = append(args, "--results-only", "--select", strings.Join(fields, ","))
					}
					handler := func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						switch command {
						case "get":
							io.WriteString(w, richChatFixture)
						case "list":
							io.WriteString(w, `{"messages":[`+richChatFixture+`]}`)
						case "search":
							io.WriteString(w, `{"results":[{"message":`+richChatFixture+`} ]}`)
						}
					}
					var result executeTestResult
					if command == "search" {
						result = executeWithChatSearchTestService(t, args, newChatSearchTestClient(t, handler))
					} else {
						result = executeWithChatTestService(t, args, useFakeChatService(t, handler))
					}
					if result.err != nil {
						t.Fatal(result.err)
					}
					var decoded any
					if err := json.Unmarshal([]byte(result.stdout), &decoded); err != nil {
						t.Fatal(err)
					}
					for _, want := range []string{"hostile-card-1", "hostile-thread-1", "client-hostile", "spaces/FORWARDED", "/attachments/qa1"} {
						if !strings.Contains(result.stdout, want) {
							t.Errorf("missing SDK field %q", want)
						}
					}
					assertChatStringsWrapped(t, decoded, wrap)
				})
			}
		}
	}
}

func assertChatStringsWrapped(t *testing.T, value any, wrap bool) {
	t.Helper()
	switch v := value.(type) {
	case map[string]any:
		for _, child := range v {
			assertChatStringsWrapped(t, child, wrap)
		}
	case []any:
		for _, child := range v {
			assertChatStringsWrapped(t, child, wrap)
		}
	case string:
		marker := strings.Contains(v, "<<<EXTERNAL_UNTRUSTED_CONTENT")
		if strings.Contains(v, "hostile") && marker != wrap {
			t.Errorf("wrapped=%v want=%v: %s", marker, wrap, v)
		}
		if (strings.Contains(v, "https://example.com/") || strings.Contains(v, "spaces/AAA") || strings.Contains(v, "spaces/FORWARDED") || strings.Contains(v, "users/123")) && marker {
			t.Errorf("usable metadata wrapped: %s", v)
		}
	}
}

func TestChatMessageGetPlainSummary(t *testing.T) {
	svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, richChatFixture)
	})
	result := executeWithChatTestService(t, []string{"--account", "a@example.com", "--plain", "chat", "messages", "get", "spaces/AAA/messages/m1"}, svc)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if !strings.Contains(result.stdout, "spaces/AAA/messages/m1") || !strings.Contains(result.stdout, "hostile text") || strings.Contains(result.stdout, "cardsV2") {
		t.Fatal(result.stdout)
	}
}

func TestChatFullProviderDefaults(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		svc := useFakeChatService(t, func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Has("markupSyntax") {
				t.Error("default markup must remain unset")
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"messages":[null,{"name":"spaces/AAA/messages/card-only","cards":[{"header":{"title":"card only"}}]}]}`)
		})
		result := executeWithChatTestService(t, []string{"--account", "a@example.com", "--json", "chat", "messages", "list", "AAA", "--full"}, svc)
		if result.err != nil {
			t.Fatal(result.err)
		}
		var output map[string]json.RawMessage
		if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
			t.Fatal(err)
		}
		if len(output) != 2 || output["messages"] == nil || output["nextPageToken"] == nil {
			t.Fatalf("list envelope changed: %s", result.stdout)
		}
	})
	t.Run("search", func(t *testing.T) {
		svc := newChatSearchTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			var request chat.SearchMessagesRequest
			json.NewDecoder(r.Body).Decode(&request)
			if request.View != "SEARCH_MESSAGES_VIEW_BASIC" || request.MarkupSyntax != "" || request.PageToken != "start" {
				t.Errorf("full output changed provider defaults: %+v", request)
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"results":[{"message":{"name":"spaces/AAA/messages/m1","text":"hello"}}],"nextPageToken":"next"}`)
		})
		result := executeWithChatSearchTestService(t, []string{"--account", "a@example.com", "--json", "chat", "messages", "search", "query", "--full", "--page", "start"}, svc)
		if result.err != nil {
			t.Fatal(result.err)
		}
		if strings.Contains(result.stdout, `"read"`) || !strings.Contains(result.stdout, `"nextPageToken": "next"`) {
			t.Fatal(result.stdout)
		}
	})
}
