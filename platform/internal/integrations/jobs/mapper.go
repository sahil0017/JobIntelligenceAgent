package jobs

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"careerpilot/contracts/domain"
)

var (
	scriptStylePattern = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>|<style\b[^>]*>.*?</style\s*>`)
	htmlTagPattern     = regexp.MustCompile(`<[^>]*>`)
	spacePattern       = regexp.MustCompile(`\s+`)
	resumeWordPattern  = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+#.]{1,}`)
)

func MapJob(source domain.CareerSource, externalID, title, location, listingURL, description, posted string, profile domain.CandidateProfile) (domain.Job, bool) {
	title = strings.TrimSpace(title)
	externalID = strings.TrimSpace(externalID)
	parsedURL, err := url.Parse(strings.TrimSpace(listingURL))
	if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || title == "" || externalID == "" {
		return domain.Job{}, false
	}
	location = strings.TrimSpace(location)
	if location == "" {
		location = "Location not listed"
	}
	score, tags := ScoreJob(title, CleanHTML(description), location, profile)

	providerLabel := "Greenhouse"
	if source.Provider == domain.ProviderLever {
		providerLabel = "Lever"
	}
	logo := "?"
	if company := []rune(source.Company); len(company) > 0 {
		logo = strings.ToUpper(string(company[0]))
	}
	return domain.Job{
		ID:          fmt.Sprintf("%s:%s:%s", source.Provider, source.BoardID, externalID),
		Company:     source.Company,
		Role:        title,
		Location:    location,
		WorkMode:    WorkMode(location),
		Salary:      "Not listed",
		Posted:      FormatPostedDate(posted),
		Source:      providerLabel,
		SourceID:    source.Provider,
		SourceBoard: source.BoardID,
		URL:         parsedURL.String(),
		Score:       score,
		Tags:        tags,
		Status:      "New",
		Logo:        logo,
		LogoTone:    "blue",
	}, true
}

func CleanHTML(content string) string {
	content = scriptStylePattern.ReplaceAllString(content, " ")
	content = htmlTagPattern.ReplaceAllString(content, " ")
	content = html.UnescapeString(content)
	return strings.TrimSpace(spacePattern.ReplaceAllString(content, " "))
}

func FormatPostedDate(posted string) string {
	if posted == "" {
		return "Date not listed"
	}
	timestamp, err := time.Parse(time.RFC3339, posted)
	if err != nil {
		return "Date not listed"
	}
	days := int(time.Since(timestamp).Hours() / 24)
	switch {
	case days <= 0:
		return "Today"
	case days == 1:
		return "1 day ago"
	case days < 30:
		return fmt.Sprintf("%d days ago", days)
	default:
		return timestamp.Local().Format("Jan 2, 2006")
	}
}

func WorkMode(location string) string {
	switch {
	case strings.Contains(strings.ToLower(location), "remote"), strings.Contains(strings.ToLower(location), "distributed"):
		return "Remote"
	case strings.Contains(strings.ToLower(location), "hybrid"):
		return "Hybrid"
	default:
		return "Not specified"
	}
}

func ScoreJob(title, description, location string, profile domain.CandidateProfile) (int, []string) {
	content := strings.ToLower(title + " " + description)
	skills := splitSkills(profile.Skills)
	matchedSkills := make([]string, 0, len(skills))
	for _, skill := range skills {
		if strings.Contains(content, strings.ToLower(skill)) {
			matchedSkills = append(matchedSkills, skill)
		}
	}

	roleWords := splitWords(profile.Title)
	roleMatches := 0
	for _, word := range roleWords {
		if strings.Contains(strings.ToLower(title), word) {
			roleMatches++
		}
	}

	var score float64
	if len(skills) > 0 {
		score += float64(len(matchedSkills)) / float64(len(skills)) * 65
	}
	if len(roleWords) > 0 {
		score += float64(roleMatches) / float64(len(roleWords)) * 25
	}
	if preferred := strings.ToLower(strings.TrimSpace(profile.Location)); preferred != "" &&
		(strings.Contains(strings.ToLower(location), preferred) || strings.Contains(strings.ToLower(location), "remote")) {
		score += 10
	}
	resumeMatches := matchingResumeKeywords(title+" "+description, profile.ResumeText)
	if len(resumeMatches) > 0 {
		score += float64(min(len(resumeMatches)*5, 20))
		for _, match := range resumeMatches {
			if len(matchedSkills) == 4 {
				break
			}
			if !containsFold(matchedSkills, match) {
				matchedSkills = append(matchedSkills, match)
			}
		}
	}
	if len(matchedSkills) > 4 {
		matchedSkills = matchedSkills[:4]
	}
	return min(100, int(score+0.5)), matchedSkills
}

var resumeStopWords = map[string]struct{}{
	"about": {}, "after": {}, "also": {}, "and": {}, "any": {}, "are": {}, "as": {}, "at": {},
	"been": {}, "before": {}, "being": {}, "both": {}, "but": {}, "by": {}, "can": {}, "career": {},
	"company": {}, "currently": {}, "for": {}, "from": {}, "have": {}, "into": {}, "its": {}, "job": {},
	"more": {}, "most": {}, "not": {}, "one": {}, "our": {}, "out": {}, "over": {}, "role": {}, "she": {},
	"should": {}, "since": {}, "such": {}, "than": {}, "that": {}, "the": {}, "their": {}, "them": {},
	"then": {}, "there": {}, "these": {}, "they": {}, "this": {}, "those": {}, "through": {}, "under": {},
	"using": {}, "was": {}, "were": {}, "with": {}, "within": {}, "work": {}, "worked": {}, "working": {},
	"year": {}, "years": {}, "your": {}, "http": {}, "https": {}, "www": {}, "com": {}, "dev": {},
	"email": {}, "phone": {}, "address": {}, "linkedin": {}, "github": {}, "resume": {}, "summary": {},
	"experience": {}, "skills": {}, "education": {}, "university": {}, "project": {}, "projects": {},
}

func matchingResumeKeywords(jobText, resumeText string) []string {
	jobTerms := make(map[string]struct{})
	for _, word := range resumeWordPattern.FindAllString(strings.ToLower(jobText), -1) {
		jobTerms[word] = struct{}{}
	}
	matches := make([]string, 0, 4)
	seen := make(map[string]struct{})
	for _, word := range resumeWordPattern.FindAllString(strings.ToLower(resumeText), -1) {
		if len(word) < 3 && word != "go" && word != "c#" {
			continue
		}
		if _, skip := resumeStopWords[word]; skip {
			continue
		}
		if _, duplicate := seen[word]; duplicate {
			continue
		}
		if _, found := jobTerms[word]; !found {
			continue
		}
		seen[word] = struct{}{}
		matches = append(matches, word)
		if len(matches) == 4 {
			break
		}
	}
	return matches
}

func containsFold(values []string, value string) bool {
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return true
		}
	}
	return false
}

func splitSkills(skills string) []string {
	parts := strings.FieldsFunc(skills, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	seen := make(map[string]struct{})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := strings.ToLower(part)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, part)
	}
	return result
}

func splitWords(value string) []string {
	value = strings.NewReplacer("/", " ", "-", " ").Replace(value)
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '+' || r == '#' || r == '.')
	})
	seen := make(map[string]struct{})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		if len([]rune(part)) <= 2 {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}
