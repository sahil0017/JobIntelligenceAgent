package greenhouse

type boardResponse struct {
	Jobs []posting `json:"jobs"`
}

type posting struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	AbsoluteURL    string `json:"absolute_url"`
	Content        string `json:"content"`
	FirstPublished string `json:"first_published"`
}
