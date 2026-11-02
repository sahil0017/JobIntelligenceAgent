package lever

type posting struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	Categories struct {
		Location string `json:"location"`
	} `json:"categories"`
	HostedURL        string `json:"hostedUrl"`
	Description      string `json:"description"`
	DescriptionPlain string `json:"descriptionPlain"`
	CreatedAt        int64  `json:"createdAt"`
}
