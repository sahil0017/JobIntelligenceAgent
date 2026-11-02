package jobs

import (
	"reflect"
	"testing"

	"careerpilot/contracts/domain"
)

func TestMapJobScoresProfileAndRejectsUnsafeListingURL(t *testing.T) {
	source := domain.CareerSource{Provider: domain.ProviderGreenhouse, Company: "Example", BoardID: "example"}
	profile := domain.CandidateProfile{
		Skills:   "Java, Spring Boot, Kafka",
		Title:    "Backend Engineer",
		Location: "Bengaluru, India",
	}
	job, ok := MapJob(source, "42", "Backend Engineer", "Bengaluru, India", "https://jobs.example/42", "<p>Java, Spring Boot and Kafka.</p>", "2026-10-29T10:00:00Z", profile)
	if !ok {
		t.Fatal("valid posting was not mapped")
	}
	if job.Score != 100 || !reflect.DeepEqual(job.Tags, []string{"Java", "Spring Boot", "Kafka"}) {
		t.Fatalf("unexpected match score: %#v", job)
	}
	if _, ok := MapJob(source, "1", "Engineer", "Remote", "javascript:alert(1)", "", "", profile); ok {
		t.Fatal("unsafe listing URL was accepted")
	}
}

func TestScoreJobUsesSavedResumeKeywords(t *testing.T) {
	profile := domain.CandidateProfile{
		ResumeText: "Built scalable Go services with Kubernetes, PostgreSQL and Kafka for high-volume systems.",
	}
	score, tags := ScoreJob("Backend Engineer", "Build Go services, Kafka pipelines, and Kubernetes workloads.", "Bengaluru", profile)
	if score < 15 {
		t.Fatalf("resume terms did not meaningfully affect the match score: %d", score)
	}
	for _, expected := range []string{"go", "kubernetes", "kafka"} {
		if !containsFold(tags, expected) {
			t.Errorf("resume match evidence %q missing from tags: %#v", expected, tags)
		}
	}
}

func TestCleanHTMLRemovesScriptsAndDecodesEntities(t *testing.T) {
	got := CleanHTML(`<p>Java &amp; Go</p><script>do not include</script>`)
	if got != "Java & Go" {
		t.Fatalf("unexpected cleaned description %q", got)
	}
}
