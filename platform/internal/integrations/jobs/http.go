package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxBoardResponseSize = 20 << 20

func FetchJSON(ctx context.Context, client *http.Client, endpoint, providerName, company string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("create %s request: %w", providerName, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "CareerPilotAI/0.1 (local job discovery)")

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("%s (%s): %w", company, providerName, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("%s (%s) returned HTTP %d", company, providerName, response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxBoardResponseSize+1))
	if err != nil {
		return fmt.Errorf("read %s response: %w", providerName, err)
	}
	if len(body) > maxBoardResponseSize {
		return fmt.Errorf("%s response exceeds the %d MiB size limit", providerName, maxBoardResponseSize>>20)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode %s response: %w", providerName, err)
	}
	return nil
}

func ValidateBoardID(boardID string) error {
	if !boardIDPattern.MatchString(strings.TrimSpace(boardID)) {
		return fmt.Errorf("board ID may contain only letters, numbers, dots, underscores, and hyphens")
	}
	return nil
}
