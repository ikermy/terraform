package entity

type Job struct {
	ID       string
	Name     string
	Command  string
	Status   string // computed by the server, e.g. "queued"
	Priority int
}
