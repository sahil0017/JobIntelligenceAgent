package linkedin

import "strings"

func mapProfile(response Profile) Profile {
	response.ID = strings.TrimSpace(response.ID)
	response.Name = strings.TrimSpace(response.Name)
	response.GivenName = strings.TrimSpace(response.GivenName)
	response.FamilyName = strings.TrimSpace(response.FamilyName)
	response.Email = strings.TrimSpace(response.Email)
	response.Picture = strings.TrimSpace(response.Picture)
	if response.Name == "" {
		response.Name = strings.TrimSpace(response.GivenName + " " + response.FamilyName)
	}
	return response
}
