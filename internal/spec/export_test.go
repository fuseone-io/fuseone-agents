package spec

// PauseAfterJudging holds every publication of p between judging the event
// graph and writing the version, by calling hold there.
func PauseAfterJudging(p *Publisher, hold func()) *Publisher {
	p.afterJudging = hold
	return p
}
