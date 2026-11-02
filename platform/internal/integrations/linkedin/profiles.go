package linkedin

import (
	"context"
	"fmt"
)

type Profile struct {
	ID         string `json:"sub"`
	Name       string `json:"name"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Email      string `json:"email"`
	Picture    string `json:"picture"`
}

func (client *Client) GetProfile(ctx context.Context) (Profile, error) {
	var profile Profile
	if err := client.getJSON(ctx, "/v2/userinfo", nil, &profile); err != nil {
		return Profile{}, fmt.Errorf("get LinkedIn profile: %w", err)
	}
	return mapProfile(profile), nil
}
