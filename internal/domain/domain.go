package domain

import "time"

type Asset struct {
	Symbol string
	Name   string
}

type SignalDirection string

const (
	DirectionLong  SignalDirection = "long"
	DirectionShort SignalDirection = "short"
	DirectionHold  SignalDirection = "hold"
)

const (
	IndicatorRSI       = "rsi"
	IndicatorMACD      = "macd"
	IndicatorBollinger = "bollinger"
	IndicatorVolumeZ   = "volume_zscore"
)

type Signal struct {
	ID        int64           `json:"id"`
	Symbol    string          `json:"symbol"`
	Interval  string          `json:"interval"`
	Indicator string          `json:"indicator"`
	Timestamp time.Time       `json:"timestamp"`
	Risk      RiskLevel       `json:"risk"`
	Direction SignalDirection `json:"direction"`
	Details   string          `json:"details,omitempty"`
	Image     *SignalImageRef `json:"image,omitempty"`
}

type SignalImageRef struct {
	ImageID   int64     `json:"image_id"`
	MimeType  string    `json:"mime_type"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SignalImageData struct {
	Ref   SignalImageRef
	Bytes []byte
}

type SignalFilter struct {
	Symbol    string
	Risk      *RiskLevel
	Indicator string
	Limit     int
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

func (r RiskLevel) IsValid() bool {
	return r >= RiskLevel1 && r <= RiskLevel5
}

type ConversationMessage struct {
	Role      string
	Content   string
	CreatedAt time.Time
}
