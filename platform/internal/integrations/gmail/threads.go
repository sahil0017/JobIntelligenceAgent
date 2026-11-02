package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"mime"
	"net/http"
	"net/mail"
	"strings"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

const (
	maxThreadResults  = 20
	maxThreadMessages = 10
	maxMessageBody    = 8000
)

type ThreadSummary struct {
	ID       string           `json:"id"`
	Snippet  string           `json:"snippet"`
	Messages []MessageSummary `json:"messages"`
}

type MessageSummary struct {
	ID      string `json:"id"`
	From    string `json:"from"`
	Subject string `json:"subject"`
	Date    string `json:"date"`
	Snippet string `json:"snippet"`
	Body    string `json:"body"`
}

func SearchThreads(ctx context.Context, client *http.Client, query string, limit int) ([]ThreadSummary, error) {
	query = strings.TrimSpace(query)
	if len(query) > 500 {
		return nil, fmt.Errorf("Gmail search query must not exceed 500 characters")
	}
	if limit < 1 || limit > maxThreadResults {
		return nil, fmt.Errorf("Gmail result limit must be between 1 and %d", maxThreadResults)
	}
	if client == nil {
		return nil, fmt.Errorf("Gmail is not connected")
	}

	service, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create Gmail API client: %w", err)
	}
	list, err := service.Users.Threads.List("me").Q(query).MaxResults(int64(limit)).Do()
	if err != nil {
		return nil, fmt.Errorf("search Gmail threads: %w", err)
	}

	results := make([]ThreadSummary, 0, len(list.Threads))
	for _, thread := range list.Threads {
		if thread == nil || thread.Id == "" {
			continue
		}
		details, err := service.Users.Threads.Get("me", thread.Id).
			Format("full").
			Do()
		if err != nil {
			return nil, fmt.Errorf("retrieve Gmail thread metadata: %w", err)
		}
		if details == nil {
			return nil, fmt.Errorf("Gmail returned an empty thread response")
		}
		summary := ThreadSummary{ID: details.Id, Snippet: details.Snippet}
		messages := details.Messages
		if len(messages) > maxThreadMessages {
			messages = messages[len(messages)-maxThreadMessages:]
		}
		for _, message := range messages {
			if message == nil {
				continue
			}
			item := MessageSummary{ID: message.Id, Snippet: message.Snippet}
			if message.Payload != nil {
				item.Body = messageBody(message.Payload)
				for _, header := range message.Payload.Headers {
					if header == nil {
						continue
					}
					switch strings.ToLower(header.Name) {
					case "from":
						item.From = header.Value
					case "subject":
						item.Subject = header.Value
					case "date":
						item.Date = header.Value
					}
				}
			}
			summary.Messages = append(summary.Messages, item)
		}
		results = append(results, summary)
	}
	return results, nil
}

func SendMessage(ctx context.Context, client *http.Client, recipient, subject, body string) (string, error) {
	recipient = strings.TrimSpace(recipient)
	parsedRecipient, err := mail.ParseAddress(recipient)
	if err != nil || parsedRecipient.Address == "" || parsedRecipient.Address != recipient ||
		strings.ContainsAny(recipient, "\r\n") {
		return "", fmt.Errorf("recipient must be a single valid email address")
	}
	if strings.TrimSpace(subject) == "" || len(subject) > 200 || strings.ContainsAny(subject, "\r\n") {
		return "", fmt.Errorf("subject must be between 1 and 200 characters and contain no line breaks")
	}
	if strings.TrimSpace(body) == "" || len(body) > 10000 {
		return "", fmt.Errorf("message body must be between 1 and 10000 characters")
	}
	if client == nil {
		return "", fmt.Errorf("Gmail is not connected")
	}

	service, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("create Gmail API client: %w", err)
	}
	encodedSubject := mime.QEncoding.Encode("UTF-8", subject)
	messageText := strings.Join([]string{
		"To: " + parsedRecipient.Address,
		"Subject: " + encodedSubject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n"), "\n", "\r\n"),
	}, "\r\n")
	raw := base64.RawURLEncoding.EncodeToString([]byte(messageText))
	sent, err := service.Users.Messages.Send("me", &gmail.Message{Raw: raw}).Do()
	if err != nil {
		return "", fmt.Errorf("send Gmail message: %w", err)
	}
	if sent == nil || sent.Id == "" {
		return "", fmt.Errorf("Gmail did not return a sent message ID")
	}
	return sent.Id, nil
}

func GmailAddress(ctx context.Context, client *http.Client) (string, error) {
	if client == nil {
		return "", fmt.Errorf("Gmail is not connected")
	}
	service, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return "", fmt.Errorf("create Gmail API client: %w", err)
	}
	profile, err := service.Users.GetProfile("me").Do()
	if err != nil {
		return "", fmt.Errorf("get connected Gmail address: %w", err)
	}
	if profile == nil || strings.TrimSpace(profile.EmailAddress) == "" {
		return "", fmt.Errorf("Gmail did not return the connected account address")
	}
	return profile.EmailAddress, nil
}

func messageBody(part *gmail.MessagePart) string {
	if part == nil {
		return ""
	}
	if strings.EqualFold(part.MimeType, "text/plain") && part.Body != nil && part.Body.Data != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(part.Body.Data)
		if err == nil {
			return truncateBody(string(decoded))
		}
	}
	for _, child := range part.Parts {
		if body := messageBody(child); body != "" {
			return body
		}
	}
	if strings.EqualFold(part.MimeType, "text/html") && part.Body != nil && part.Body.Data != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(part.Body.Data)
		if err == nil {
			return truncateBody(htmlText(string(decoded)))
		}
	}
	return ""
}

func htmlText(content string) string {
	document, err := xhtml.Parse(strings.NewReader(content))
	if err != nil {
		return html.UnescapeString(content)
	}
	var text strings.Builder
	var visit func(*xhtml.Node)
	visit = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && (node.Data == "script" || node.Data == "style") {
			return
		}
		if node.Type == xhtml.TextNode {
			text.WriteString(node.Data)
			text.WriteByte(' ')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return strings.Join(strings.Fields(text.String()), " ")
}

func truncateBody(body string) string {
	body = strings.TrimSpace(body)
	if !utf8.ValidString(body) {
		body = strings.ToValidUTF8(body, "")
	}
	if len(body) <= maxMessageBody {
		return body
	}
	end := maxMessageBody
	for end > 0 && !utf8.RuneStart(body[end]) {
		end--
	}
	return body[:end] + "…"
}
