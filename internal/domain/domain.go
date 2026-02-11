package domain

type Asset struct {
	Symbol string
	Name   string
}

type Signal struct {
	Asset     Asset
	Indicator string
	Timestamp int64
	Risk      RiskLevel
	Direction string // long, short, hold
}

type Recommendation struct {
	Signal Signal
	Text   string
}

type RiskLevel int

const (
	RiskLevel1 RiskLevel = 1
	RiskLevel2 RiskLevel = 2
	RiskLevel3 RiskLevel = 3
	RiskLevel4 RiskLevel = 4
	RiskLevel5 RiskLevel = 5
)
